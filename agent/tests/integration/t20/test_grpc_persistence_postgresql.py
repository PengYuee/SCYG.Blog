"""Generated AgentControl 到 PostgreSQL 的重启持久化链验收。"""

import grpc
import pytest
from cryptography.hazmat.primitives.asymmetric import rsa
from tests.adapters.auth.test_jwt_verifier import claims, encode
from tests.integration.t20.postgres_support import (
    T20Database,
    grpc_stub,
    migrate,
    migration_environment,
)

from scyg_agent.adapters.auth import JwtVerifier
from scyg_agent.adapters.database.run_request_source import (
    PersistedRunRequestSource,
    PostgreSQLRunInputSource,
)
from scyg_agent.domain.runs import RunId
from scyg_agent.domain.runs.input import MissingRunInput, RunInput
from scyg_agent.generated.scyg.agent.v1 import agent_control_service_pb2 as service_pb2
from scyg_agent.generated.scyg.agent.v1 import common_pb2


def request(
    message: str = "持久化输入", article: str = "article-20"
) -> service_pb2.CreateRunRequest:
    return service_pb2.CreateRunRequest(
        metadata=common_pb2.RequestMetadata(request_id="request-1", correlation_id="trace-1"),
        operation_id="t20:persisted:create",
        owner_user_id=common_pb2.UserId(value="user-t20"),
        task_type=common_pb2.TASK_TYPE_SUMMARY,
        runtime=common_pb2.RuntimeSelection(kind=common_pb2.RUNTIME_KIND_SIMPLE, version="v1"),
        initial_message=message,
        article_id=common_pb2.ArticleId(value=article),
    )


@pytest.mark.anyio
async def test_generated_create_survives_reconstruction_and_conflicts_without_mutation(
    verifier: JwtVerifier, private_key: rsa.RSAPrivateKey
) -> None:
    environment = migration_environment()
    migrate(["downgrade", "base"], environment)
    migrate(["upgrade", "head"], environment)
    first_database = T20Database.create(
        environment["SCYG_AGENT_DATABASE_URL"], environment["SCYG_T20_LISTENER_DSN"]
    )
    metadata = (("authorization", f"Bearer {encode(private_key, claims('blog_service')).value}"),)
    try:
        await first_database.reset()
        async with grpc_stub(first_database.facade(), verifier) as stub:
            first = await stub.CreateRun(request(), metadata=metadata)
        await first_database.close()
        run_id = RunId(first.run.id.value)
        database = T20Database.create(
            environment["SCYG_AGENT_DATABASE_URL"], environment["SCYG_T20_LISTENER_DSN"]
        )

        try:
            async with grpc_stub(database.facade(), verifier) as rebuilt:
                replay = await rebuilt.CreateRun(request(), metadata=metadata)
                for changed in (request(message="不同输入"), request(article="article-other")):
                    with pytest.raises(grpc.aio.AioRpcError) as captured:
                        _ = await rebuilt.CreateRun(changed, metadata=metadata)
                    assert captured.value.code() is grpc.StatusCode.ALREADY_EXISTS

            persisted = await PostgreSQLRunInputSource(database.sessions).get_input(run_id)
            assert isinstance(persisted, RunInput)
            provider = await PersistedRunRequestSource(
                PostgreSQLRunInputSource(database.sessions), "test-model"
            ).request_for(await database.run(run_id))
            assert replay.run.id == first.run.id
            assert persisted.initial_message == "持久化输入"
            assert persisted.article_id == "article-20"
            assert provider.messages[0].content == "持久化输入"
            assert await database.counts() == (1, 0, 0, 0)
            await database.clear_input(run_id)
            historical = await PostgreSQLRunInputSource(database.sessions).get_input(run_id)
            assert isinstance(historical, MissingRunInput)
        finally:
            await database.close()
    finally:
        await first_database.close()
        migrate(["downgrade", "base"], environment)
