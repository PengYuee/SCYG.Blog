"""应用门面闭合结果映射测试。"""

import pytest

from scyg_agent.application import (
    FacadeConflict,
    FacadeInternal,
    FacadeNotFound,
    FacadePrecondition,
    InvalidFacadeInputError,
    OwnedCommand,
    OwnerContext,
)
from scyg_agent.domain.ports.command_store import (
    CommandApplyResult,
    CommandDataIntegrity,
    CommandRejected,
    CommandRunNotFound,
    IdempotencyConflict,
    UnsupportedCommand,
)
from scyg_agent.domain.runs import CancelRun, CommandId, CommandKind, EventId, RunStatus, UserId

from .test_facade import NOW, OWNER, RUN_ID, FakeCommands, facade, make_run, submission


@pytest.mark.anyio
@pytest.mark.parametrize(
    ("stored", "expected"),
    [
        (CommandRejected("revision_mismatch", 1, 0, replayed=False), FacadePrecondition),
        (IdempotencyConflict(CommandId("cmd_t20cancel0")), FacadeConflict),
        (CommandRunNotFound(RUN_ID), FacadeNotFound),
        (UnsupportedCommand(CommandId("cmd_t20cancel0"), CommandKind.CANCEL), FacadePrecondition),
        (CommandDataIntegrity(CommandId("cmd_t20cancel0")), FacadeInternal),
    ],
)
async def test_command_store_outcomes_are_closed(
    stored: CommandApplyResult,
    expected: type[FacadePrecondition]
    | type[FacadeConflict]
    | type[FacadeNotFound]
    | type[FacadeInternal],
) -> None:
    app, _, _ = facade(make_run(RunStatus.WAITING_INPUT), commands=FakeCommands(result=stored))
    command = CancelRun(CommandId("cmd_t20cancel0"), EventId("evt_t20cancel0"), 1, NOW)
    outcome = await app.submit_command(OwnedCommand(OwnerContext(OWNER), submission(command)))
    assert isinstance(outcome, expected)


def test_exact_owner_type_is_rejected() -> None:
    with pytest.raises(InvalidFacadeInputError, match="应用请求字段无效"):
        _ = OwnerContext(UserIdChild("user-t20"))


class UserIdChild(UserId):
    """构造精确类型拒绝探针。"""


@pytest.fixture
def anyio_backend() -> str:
    return "asyncio"
