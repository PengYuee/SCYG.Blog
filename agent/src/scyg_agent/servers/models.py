"""服务监听器共享的类型化生命周期值."""

from dataclasses import dataclass
from enum import StrEnum
from typing import override


class ServerState(StrEnum):
    """限定服务组件允许公开的生命周期状态."""

    NEW = "new"
    STARTING = "starting"
    RUNNING = "running"
    STOPPING = "stopping"
    STOPPED = "stopped"
    FAILED = "failed"


@dataclass(frozen=True, slots=True)
class BoundEndpoint:
    """仅公开可连接的实际地址,不保留配置对象或秘密."""

    host: str
    port: int


@dataclass(frozen=True, slots=True)
class ServerStartError(RuntimeError):
    """以稳定中文消息报告监听器绑定或启动失败."""

    component: str

    @override
    def __str__(self) -> str:
        """返回不含绑定配置的稳定中文消息."""
        return f"{self.component} 服务绑定或启动失败"
