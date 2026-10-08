from collections.abc import Iterable, Sequence
from typing import Protocol

from google.protobuf.any_pb2 import Any
from google.protobuf.message import Message

class PackedDetail(Protocol):
    @property
    def type_url(self) -> str: ...
    @property
    def value(self) -> bytes: ...
    def Unpack(self, msg: Message) -> bool: ...

class Status(Message):
    code: int
    message: str
    @property
    def details(self) -> Sequence[PackedDetail]: ...
    def __init__(
        self,
        code: int | None = ...,
        message: str | None = ...,
        details: Iterable[Any] | None = ...,
    ) -> None: ...
