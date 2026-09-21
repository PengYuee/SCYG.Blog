"""Generated protobuf transport contract tests."""

from typing import TYPE_CHECKING

from google.protobuf import descriptor_pool

if TYPE_CHECKING:
    from google.protobuf.descriptor import ServiceDescriptor

from scyg_agent.generated.scyg.agent.v1 import (
    agent_control_service_pb2,
    agent_control_service_pb2_grpc,
)
from scyg_agent.generated.scyg.agent.v1 import common_pb2 as agent_common_pb2
from scyg_agent.generated.scyg.blog.v1 import (
    blog_tool_service_pb2,
    blog_tool_service_pb2_grpc,
)
from scyg_agent.generated.scyg.blog.v1 import common_pb2 as blog_common_pb2


def test_agent_requests_round_trip_through_generated_messages() -> None:
    # Given: representative create and lookup requests at the gRPC boundary.
    create_request = agent_control_service_pb2.CreateRunRequest(
        metadata=agent_common_pb2.RequestMetadata(
            request_id="request-create-1",
            correlation_id="correlation-1",
        ),
        operation_id="operation-create-1",
        owner_user_id=agent_common_pb2.UserId(value="user-1"),
        task_type=agent_common_pb2.TASK_TYPE_SUMMARY,
        runtime=agent_common_pb2.RuntimeSelection(
            kind=agent_common_pb2.RUNTIME_KIND_SIMPLE,
            version="v1",
        ),
        initial_message="Summarize the article.",
    )
    get_request = agent_control_service_pb2.GetRunRequest(
        metadata=agent_common_pb2.RequestMetadata(
            request_id="request-get-1",
            correlation_id="correlation-1",
        ),
        run_id=agent_common_pb2.RunId(value="run_12345678"),
        owner_user_id=agent_common_pb2.UserId(value="user-1"),
    )

    # When: protobuf wire bytes are serialized and parsed by generated classes.
    parsed_create = agent_control_service_pb2.CreateRunRequest.FromString(
        create_request.SerializeToString()
    )
    parsed_get = agent_control_service_pb2.GetRunRequest.FromString(get_request.SerializeToString())

    # Then: representative boundary values survive the wire round trip.
    assert parsed_create == create_request
    assert parsed_get == get_request


def test_blog_tool_request_and_response_round_trip() -> None:
    # Given: one representative Blog tool request and response.
    request = blog_tool_service_pb2.SearchArticlesRequest(
        metadata=blog_common_pb2.ToolRequestMetadata(
            request_id="request-search-1",
            correlation_id="correlation-1",
            run_id="run_12345678",
            tool_call_id="tool_12345678",
        ),
        query="protobuf contracts",
        page_size=20,
    )
    response = blog_tool_service_pb2.SearchArticlesResponse()

    # When: both messages cross their generated protobuf wire boundary.
    parsed_request = blog_tool_service_pb2.SearchArticlesRequest.FromString(
        request.SerializeToString()
    )
    parsed_response = blog_tool_service_pb2.SearchArticlesResponse.FromString(
        response.SerializeToString()
    )

    # Then: request and response values remain stable.
    assert parsed_request == request
    assert parsed_response == response


def test_generated_descriptors_and_stubs_match_contract_services() -> None:
    # Given: generated service descriptors and importable stub classes.
    agent_service: ServiceDescriptor = descriptor_pool.Default().FindServiceByName(
        "scyg.agent.v1.AgentControlService"
    )
    blog_service: ServiceDescriptor = descriptor_pool.Default().FindServiceByName(
        "scyg.blog.v1.BlogToolService"
    )

    # When: method names and generated stub types are inspected without a channel.
    agent_methods = tuple(method.name for method in agent_service.methods)
    blog_methods = tuple(method.name for method in blog_service.methods)
    stub_types = (
        agent_control_service_pb2_grpc.AgentControlServiceStub,
        blog_tool_service_pb2_grpc.BlogToolServiceStub,
    )

    # Then: both T04 service surfaces are preserved exactly.
    assert agent_service.full_name == "scyg.agent.v1.AgentControlService"
    assert agent_methods == ("CreateRun", "CreateAgentRun", "GetRun")
    assert blog_service.full_name == "scyg.blog.v1.BlogToolService"
    assert blog_methods == (
        "GetPublishedArticle",
        "SearchArticles",
        "CreateArticleDraft",
        "UpdateArticleDraft",
        "PublishArticle",
        "AddArticleTags",
    )
    assert tuple(stub_type.__name__ for stub_type in stub_types) == (
        "AgentControlServiceStub",
        "BlogToolServiceStub",
    )
