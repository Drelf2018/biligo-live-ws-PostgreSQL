import ctypes


lib = ctypes.CDLL("./biligo-live-ws-PostgreSQL.so")
lib.Run()