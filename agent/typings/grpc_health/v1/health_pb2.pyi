from typing import ClassVar

from google.protobuf.message import Message

class HealthCheckRequest(Message):
    service: str
    def __init__(self, service: str | None = ...) -> None: ...

class HealthCheckResponse(Message):
    class ServingStatus(int):
        UNKNOWN: ClassVar[int]
        SERVING: ClassVar[int]
        NOT_SERVING: ClassVar[int]
        SERVICE_UNKNOWN: ClassVar[int]

    UNKNOWN: ClassVar[int]
    SERVING: ClassVar[int]
    NOT_SERVING: ClassVar[int]
    SERVICE_UNKNOWN: ClassVar[int]
    status: int
    def __init__(self, status: int | str | None = ...) -> None: ...
