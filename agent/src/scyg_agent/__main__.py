"""SCYG Agent 配置检查与生产运行命令入口."""

import asyncio  # noqa: RUF100  # noqa: ANYIO_OK - CLI 必须在 AnyIO 前配置 Windows policy。
import sys
from collections.abc import Callable, Sequence
from getpass import getpass
from typing import Final, Protocol, final

import anyio
from pydantic import ValidationError

from scyg_agent.cli_runtime import run_application
from scyg_agent.composition import ProductionApplicationFactory
from scyg_agent.config import (
    ApplicationSettings,
    ConfigurationFileError,
    load_settings,
    sanitize_validation_error,
)
from scyg_agent.local_setup import LocalSetupStageError, setup_local_database

USAGE: Final = "用法: scyg-agent [--help|--check|setup [--admin-url URL]|run]"
HELP: Final = """用法: scyg-agent [--help|--check|setup [--admin-url URL]|run]

命令:
  --help   显示帮助
  --check  验证配置和生产组件构造
  setup    创建本机数据库角色、迁移和 checkpoint
  run      启动 Agent 服务
"""
READY_MESSAGE: Final = "SCYG Agent configuration ready"
SETUP_MESSAGE: Final = "SCYG Agent database setup complete"
ServiceRunner = Callable[[ApplicationSettings], None]
SetupRunner = Callable[[ApplicationSettings, str, str | None], None]
PasswordReader = Callable[[str], str]


if sys.platform == "win32":
    from asyncio import WindowsSelectorEventLoopPolicy as _SelectorPolicy  # noqa: I001, RUF100  # noqa: ANYIO_OK
else:
    _SelectorPolicy = asyncio.DefaultEventLoopPolicy


class EventLoopPolicyApi(Protocol):
    """描述同步配置事件循环 policy 所需的最小 API."""

    def get(self) -> asyncio.AbstractEventLoopPolicy:
        """返回当前进程 policy."""
        ...

    def is_selector(self, policy: asyncio.AbstractEventLoopPolicy) -> bool:
        """判断当前 policy 是否已兼容 Selector."""
        ...

    def create_selector(self) -> asyncio.AbstractEventLoopPolicy:
        """创建 Windows Selector policy."""
        ...

    def set(self, policy: asyncio.AbstractEventLoopPolicy) -> None:
        """安装进程 policy."""
        ...


@final
class _SystemPolicyApi:
    """封装 CPython 同步事件循环 policy API."""

    def get(self) -> asyncio.AbstractEventLoopPolicy:
        """读取当前 policy."""
        return asyncio.get_event_loop_policy()

    def is_selector(self, policy: asyncio.AbstractEventLoopPolicy) -> bool:
        """检查是否已安装平台 Selector policy."""
        return isinstance(policy, _SelectorPolicy)

    def create_selector(self) -> asyncio.AbstractEventLoopPolicy:
        """创建平台守卫选择的 Selector policy."""
        return _SelectorPolicy()

    def set(self, policy: asyncio.AbstractEventLoopPolicy) -> None:
        """同步安装 policy."""
        asyncio.set_event_loop_policy(policy)


def configure_event_loop_policy(
    *, platform: str = sys.platform, api: EventLoopPolicyApi | None = None
) -> None:
    """在 Windows 异步资源创建前幂等安装 Selector policy."""
    if platform != "win32":
        return
    resolved = api or _SystemPolicyApi()
    current = resolved.get()
    if resolved.is_selector(current):
        return
    resolved.set(resolved.create_selector())


def _serve(settings: ApplicationSettings) -> None:
    """在 AnyIO asyncio 后端运行唯一生产应用."""
    configure_event_loop_policy()
    anyio.run(run_application, settings)


def _invalid_configuration(error: ValidationError) -> int:
    """输出不含输入值的配置诊断."""
    print("SCYG Agent 配置无效:", file=sys.stderr)
    for issue in sanitize_validation_error(error):
        print(f"- {issue.field}: {issue.reason}", file=sys.stderr)
    return 2


def _invalid_configuration_source(error: ValidationError | ConfigurationFileError) -> int:
    """输出类型校验或配置文件诊断."""
    if isinstance(error, ValidationError):
        return _invalid_configuration(error)
    print(f"SCYG Agent 配置文件无效: {error}", file=sys.stderr)
    return 2


def _run_setup(
    settings: ApplicationSettings,
    setup_runner: SetupRunner,
    password_reader: PasswordReader,
    setup_admin_url: str | None,
) -> int:
    """执行 setup 并隐藏凭据,同时保留安全的阶段诊断."""
    try:
        admin_password = password_reader("PostgreSQL administrator password: ")
        setup_runner(settings, admin_password, setup_admin_url)
    except LocalSetupStageError as error:
        print(f"SCYG Agent 数据库初始化失败: {error}", file=sys.stderr)
        return 1
    except Exception:  # noqa: BLE001, RUF100  # noqa: BROAD_EXCEPT_OK - CLI 边界隐藏数据库凭据与驱动错误。
        print("SCYG Agent 数据库初始化失败", file=sys.stderr)
        return 1
    print(SETUP_MESSAGE)
    return 0


def run(  # noqa: PLR0911 - 封闭 CLI 命令各自返回稳定退出码。
    arguments: Sequence[str],
    *,
    service_runner: ServiceRunner = _serve,
    setup_runner: SetupRunner = setup_local_database,
    password_reader: PasswordReader = getpass,
) -> int:
    """解析封闭命令并返回稳定进程退出码."""
    command = tuple(arguments)
    if command in {("--help",), ("-h",)}:
        print(HELP, end="")
        return 0
    setup_admin_url: str | None = None
    match command:
        case ("setup",):
            is_setup = True
        case ("setup", "--admin-url", admin_url):
            is_setup, setup_admin_url = True, admin_url
        case _:
            is_setup = False
    if command not in {("--check",), ("run",)} and not is_setup:
        print(USAGE, file=sys.stderr)
        return 2
    try:
        settings = load_settings()
    except (ValidationError, ConfigurationFileError) as error:
        return _invalid_configuration_source(error)
    if command == ("--check",):
        _ = ProductionApplicationFactory(settings).build()
        print(READY_MESSAGE)
        return 0
    if is_setup:
        return _run_setup(settings, setup_runner, password_reader, setup_admin_url)
    try:
        service_runner(settings)
    except Exception:  # noqa: BLE001, RUF100  # noqa: BROAD_EXCEPT_OK - CLI 边界隐藏配置与第三方异常。
        print("SCYG Agent 启动或关闭失败", file=sys.stderr)
        return 1
    return 0


def main() -> None:
    """使用进程参数运行 CLI."""
    raise SystemExit(run(sys.argv[1:]))


if __name__ == "__main__":
    main()
