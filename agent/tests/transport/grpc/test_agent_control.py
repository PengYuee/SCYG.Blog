"""AgentControl 生成契约的聚焦测试."""

import pytest

from scyg_agent.agents import Capability, ChatInput, SearchInput, validate_capability_input
from scyg_agent.agents.contracts import InvalidCapabilityInputError
from scyg_agent.generated.scyg.agent.v1 import agent_control_service_pb2 as service_pb2
from scyg_agent.generated.scyg.agent.v1 import agent_control_service_pb2_grpc as service_grpc
from scyg_agent.generated.scyg.agent.v1 import common_pb2
from scyg_agent.transport.grpc import AgentControlServicer
from scyg_agent.transport.grpc.conversion import (
    InvalidGrpcRequestError,
    parse_create_agent_run_request,
)


def _agent_request(
    *,
    capability: common_pb2.AgentCapability | None = None,
    search: common_pb2.SearchInput | None = None,
    chat: common_pb2.ChatInput | None = None,
) -> service_pb2.CreateAgentRunRequest:
    return service_pb2.CreateAgentRunRequest(
        metadata=common_pb2.RequestMetadata(request_id="req-1", correlation_id="corr-1"),
        idempotency_key="op-1",
        user_id=common_pb2.UserId(value="user-1"),
        locale="zh-CN",
        capability=capability,
        search=search,
        chat=chat,
    )


def test_capability_request_parser_enforces_matching_oneof() -> None:
    request = _agent_request(
        capability=common_pb2.AGENT_CAPABILITY_SEARCH,
        search=common_pb2.SearchInput(query="python", max_results=3),
    )
    user, operation, run_id, capability, value, locale = parse_create_agent_run_request(request)
    assert str(user) == "user-1"
    assert str(operation) == "op-1"
    assert str(run_id).startswith("run_")
    assert capability is Capability.SEARCH
    assert value == SearchInput(query="python", max_results=3)
    assert locale == "zh-CN"


def test_capability_request_parser_rejects_wrong_capability_pair() -> None:
    request = _agent_request(
        capability=common_pb2.AGENT_CAPABILITY_SEARCH,
        chat=common_pb2.ChatInput(message="hello"),
    )
    with pytest.raises(InvalidGrpcRequestError):
        _ = parse_create_agent_run_request(request)


def test_capability_contract_helper_rejects_wrong_python_pair() -> None:
    with pytest.raises(InvalidCapabilityInputError):
        _ = validate_capability_input(Capability.SEARCH, ChatInput(message="hello"))


def test_generated_descriptor_exposes_capability_creation_methods() -> None:
    descriptor = service_grpc.AgentControlServiceServicer
    methods = {
        name for name in ("CreateRun", "CreateAgentRun", "GetRun") if hasattr(descriptor, name)
    }
    assert methods == {"CreateRun", "CreateAgentRun", "GetRun"}
    assert not hasattr(descriptor, "BlogToolService")


def test_transport_servicer_is_importable() -> None:
    methods = {name for name in AgentControlServicer.__dict__ if not name.startswith("_")}
    assert methods == {"CreateRun", "CreateAgentRun", "GetRun"}
