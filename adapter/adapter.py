﻿import asyncio
import time
from json import dumps, loads
from logging import DEBUG, INFO, Formatter, Logger, StreamHandler

import requests
from aiowebsocket.converses import AioWebSocket
from apscheduler.schedulers.background import BackgroundScheduler

BASEURL = 'http://localhost:8080'
ROOM_STATUS = {}  # 记录开播时间
SUPER_CHAT = []  # SC的唯一id避免重复记录

class Adapter:
    '连接网页、数据库、biligo-ws-live 的适配器'
    def __init__(self, logger: Logger, aid: str, url: str = BASEURL+'/ws'):
        self.aid = aid  #  接入 biligo-ws-live 时的 id 用来区分不同监控程序
        self.url = url + f'?id={aid}' # biligo-ws-live 运行地址
        self.logger = logger  # 网页端输出 Logger
        self.converse = None  # 异步连接
        self.danmu = []  # 暂存的弹幕
        self.sched = BackgroundScheduler()  # 用来保存暂存弹幕到数据库的计时器

    async def connect(self):
        while not self.converse:  # 多次尝试连接 biligo-ws-live
            try:
                async with AioWebSocket(self.url) as aws:
                    self.converse = aws.manipulator
            except Exception:
                self.logger.info('Adapter 重连中')
                await asyncio.sleep(3)
        self.logger.info('Adapter 连接成功')
        return self.converse.receive  # 接受消息的函数

    async def run(self, listening_rooms: list):
        recv = await self.connect()
        logger = self.logger  # 不知道从哪学的减少 self. 操作耗时的方法 好像说 self. 就是查字典也要时间

        # 定时保存弹幕
        @self.sched.scheduled_job('interval', id='record_danmu', seconds=10, max_instances=3)
        def record():
            count = len(self.danmu)
            if count > 0:
                logger.info(f'储存 {count} 条弹幕记录')
                # 防止保存数据 求出现有数量后 若有新增弹幕 将新增弹幕重新存回 self.danmu
                record_danmu, self.danmu = self.danmu[:count], self.danmu[count:]
                print(record_danmu)
                # danmuDB.insert(record_danmu)

        self.sched.start()

        # 将监听房间号告知 biligo-ws-live
        requests.post(BASEURL+'/subscribe', headers={"Authorization": self.aid}, data={'subscribes': listening_rooms})

        while True:  # 死循环接受消息
            try:
                mes = await recv()
            except Exception as e:
                logger.error(e)
                break
            js = loads(mes)
            roomid = js.get('live_info', {}).get('room_id')
            if roomid in listening_rooms:
                if js['command'] == 'LIVE':  # 开播时记录时间戳在 ROOM_STATUS 里并插入数据库
                    start_time = round(time.time())
                    if not ROOM_STATUS.get(roomid):
                        ROOM_STATUS[roomid] = start_time
                        info = js['live_info']
                        name, uid, title, cover = info['name'], info['uid'], info['title'], info['cover']
                        # liveDB.insert(ROOM=roomid, USERNAME=name, UID=uid, TITLE=title, COVER=cover, ST=start_time)
                        logger.info(f'{roomid} {name} 正在直播\n标题：{title}\n封面：{cover}')

                elif js['command'] == 'DANMU_MSG':  # 接受到弹幕
                    info = js['content']['info']
                    self.danmu.append((roomid, info[9]['ts'], info[2][1], info[2][0], info[1], 'DANMU_MSG', 0, ROOM_STATUS.get(roomid, 0)))
                    logger.info(f'{roomid} {info[2][1]} {info[1]}')
                    # 向暂存弹幕库添加元组 (房间号, 时间戳, 用户名, 用户uid, 信息内容, 信息类型, 信息价值 当前直播间的开播时间)
                    # 当前直播间的开播时间 为 None 或 0 表示未开播

                elif js['command'] == 'SEND_GIFT':  # 接受到礼物
                    data = js['content']['data']
                    msg = '{action} {giftName}'.format_map(data) + f'<font color="red">￥{data["price"]/1000}</font>'
                    self.danmu.append((roomid, data['timestamp'], data['uname'], data['uid'], msg, 'SEND_GIFT', data["price"]/1000, ROOM_STATUS.get(roomid, 0)))
                    logger.info(f'{roomid} {data["uname"]} 赠送 {data["giftName"]}')

                elif js['command'] == 'GUARD_BUY':  # 接受到大航海
                    data = js['content']['data']
                    msg = f'赠送 {data["gift_name"]}<font color="red">￥{data["price"]//1000}</font>'
                    self.danmu.append((roomid, data['start_time'], data['username'], data['uid'], msg, 'GUARD_BUY', data["price"]//1000, ROOM_STATUS.get(roomid, 0)))
                    logger.info(f'{roomid} {data["username"]} 续费 {data["gift_name"]}')

                elif js['command'] in ['SUPER_CHAT_MESSAGE', 'SUPER_CHAT_MESSAGE_JPN']:  # 接受到醒目留言
                    data = js['content']['data']
                    if int(data['id']) not in SUPER_CHAT:
                        SUPER_CHAT.append(int(data['id']))
                        msg = '{message}<font color="red">￥{price}</font>'.format_map(data)
                        self.danmu.append((roomid, data['start_time'], data['user_info']['uname'], data['uid'], msg, 'SUPER_CHAT_MESSAGE', data['price'], ROOM_STATUS.get(roomid, 0)))
                        logger.info(f'{roomid} {data["user_info"]["uname"]} {msg}')

                elif js['command'] == 'PREPARING' and ROOM_STATUS.get(roomid):  # 下播 更新数据库中下播时间戳 并将全局数组清零（真能清零吗（你在暗示什么））
                    # liveDB.update(roomid, ROOM_STATUS[roomid], round(time.time()))
                    logger.info(f'{roomid} 下播了')
                    del ROOM_STATUS[roomid]

                else:
                    logger.debug(dumps(js, indent=4, ensure_ascii=False))

    async def send(self, cmd):  # 不知道有啥用的发送消息 但是既然有ws连接还是写个发送好 万一用到了呢
        if isinstance(cmd, str):
            await self.converse.send(cmd)
        else:
            try:
                js = dumps(cmd, ensure_ascii=False)
                await self.converse.send(js)
            except Exception as e:
                self.logger.error('发送失败 '+str(e))


if __name__ == '__main__':
    # 其实适配器就是个最小实例 不用运行网页也能做到保存数据了
    room_ids = [
        3762542, 21696929, 8721033, 80397,
        22300771, 510, 605, 21457197,
        21672022, 2063974, 843670, 24012881,
        23527653, 5212213, 5367, 1010,
        23550749, 22645369, 21696950, 
        23805066, 21624651, 591194, 22778610,
        22734699, 3032130, 5424, 21470454,
        13319737, 7023107, 21613353, 22778627,
        22778596, 1234455, 23260993, 238483,
        22323445, 12183395, 21302477, 23805029,
        5229, 1995809, 21403601, 47867,
        23260962, 22637261, 889434, 21672023,
        21672024, 851181, 23260932, 21919321,
        22389323, 23805078, 21677969, 22470216,
        22605463, 22470204, 2277862, 22605464,
        22605466, 22389319, 22470208, 22389314,
        22195814, 24731561, 24731543, 24731569,
        24731531, 24731541, 24375803, 213,
        9112152, 22566228, 3570974, 21443466,
        950576, 286860, 21571739, 21613356,
        21615277, 21696957, 21696953, 21763344,
        21763337, 21756924, 22111428, 22470210,
        22605469, 23017349, 21484828, 23017343,
        23017346, 23260979, 23260856, 23550793,
        23805059, 784734, 22470189, 33942,
        282208, 6068126, 14713062, 23273179,
        307757, 23104083, 22968023, 6775697,
        25206807, 23365292, 4578433, 56757,
        292397, 21742813, 21610959, 8498986,
        819440, 14703541, 876396, 25512443,
        6374209, 25788785, 25788858, 25788830,
        23593916, 5555734, 6655, 82568
    ]  # 监听中直播间号
    logger = Logger('MAIN', INFO)
    handler = StreamHandler()
    handler.setFormatter(Formatter(f'[%(asctime)s] %(message)s', '%H:%M:%S'))
    logger.addHandler(handler)
    asyncio.run(Adapter(logger, 'cha').run(room_ids))
