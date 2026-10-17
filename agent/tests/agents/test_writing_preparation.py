"""Production preparation graph and closed interaction contracts (no provider calls)."""

from __future__ import annotations

import hashlib
import json
from collections.abc import Awaitable, Callable, Mapping
from datetime import UTC, datetime, timedelta
from typing import TYPE_CHECKING, Protocol, cast, override

import pytest
from langchain.agents.middleware.types import ModelRequest, ModelResponse
from langchain_core.messages import AIMessage, HumanMessage
from langchain_openai import ChatOpenAI
from langgraph.checkpoint.memory import InMemorySaver
from langgraph.runtime import Runtime
from langgraph.types import Command, Interrupt
from pydantic import BaseModel, SecretStr, ValidationError

from scyg_agent.agents import (
    INTERACTION_REQUEST_ADAPTER,
    ArticleDraft,
    Capability,
    InvalidCapabilityOutputError,
    RecipeId,
    SelectionOption,
    SelectionRequest,
    TextInputRequest,
    WritingInput,
    input_digest,
    input_payload,
    validate_interaction_response,
)
from scyg_agent.agents.context import AgentRequestContext, Quality
from scyg_agent.agents.production import PublicModelPolicy, prepared_writing_graph
from scyg_agent.agents.public_policy import PolicyLimitError, PublicAttemptBudget
from scyg_agent.config import (
    ApplicationSettings,
    ModelProtocol,
    ModelSettings,
    ModelTierSettings,
    PublicSearchSettings,
)
from scyg_agent.domain.runs import ContentScope, UserId

if TYPE_CHECKING:
    from collections.abc import Sequence

    from langchain_core.messages import BaseMessage

    from scyg_agent.agents.production import AsyncClose


class PromptMessage(BaseModel):
    content: str


class GeneratorInput(BaseModel):
    messages: list[PromptMessage]


class ReferenceFragment(BaseModel):
    article_id: int
    fragment: str
    is_fragment: bool


class PreparedInput(BaseModel):
    requirements: str
    reference_fragments: list[ReferenceFragment]


def pending_interrupt(raw: object) -> Interrupt:
    assert isinstance(raw, Mapping)
    pending = cast("Mapping[str, object]", raw).get("__interrupt__")
    assert isinstance(pending, (list, tuple))
    entries = cast("list[object] | tuple[object, ...]", pending)
    assert len(entries) == 1
    assert isinstance(entries[0], Interrupt)
    return entries[0]


def interruption_kind(value: Interrupt) -> str:
    return INTERACTION_REQUEST_ADAPTER.validate_python(cast("object", value.value)).kind


def policy_settings(protocol: ModelProtocol) -> ApplicationSettings:
    tier = ModelTierSettings.model_validate(
        {
            "model": "public-test",
            "base_url": "https://example.test/v1",
            "api_key": "test",
            "timeout_seconds": 30,
        }
    )
    return ApplicationSettings(
        database_url=SecretStr("postgresql+asyncpg://localhost/test"),
        models=ModelSettings(protocol=protocol, fast=tier, standard=tier, strong=tier),
    )


class RequestPayloadBuilder(Protocol):
    """Narrow the real SDK payload builder to this test's messages and settings."""

    def __call__(self, messages: Sequence[BaseMessage], /, **settings: object) -> dict[str, object]:
        """Build a provider payload without making a provider request."""
        ...


class PayloadChatOpenAI(ChatOpenAI):
    """Expose real SDK payload generation through a typed subclass test seam."""

    def request_payload(self, request: ModelRequest[AgentRequestContext]) -> dict[str, object]:
        # The SDK leaves this private method's return mapping unparameterized.
        builder = cast("RequestPayloadBuilder", self.__getattribute__("_get_request_payload"))
        settings = cast("dict[str, object]", request.model_settings)
        return builder(request.messages, **settings)


@pytest.fixture
def anyio_backend() -> str:
    return "asyncio"


class Guard:
    async def dispatch[T](self, start: Callable[[], Awaitable[T]]) -> T:
        return await start()

    async def check(self) -> None:
        pass


def context(scope: ContentScope = ContentScope.AUTHOR) -> AgentRequestContext:
    return AgentRequestContext(
        UserId("author"),
        "run-preparation",
        Capability.WRITE if scope is ContentScope.AUTHOR else Capability.SEARCH,
        RecipeId.WRITING_V1 if scope is ContentScope.AUTHOR else RecipeId.SEARCH_V1,
        "v1",
        "und",
        Quality.STANDARD,
        execution_guard=Guard(),
        content_scope=scope,
        public_budget=PublicAttemptBudget(
            PublicSearchSettings(), datetime.now(UTC) + timedelta(minutes=3)
        )
        if scope is ContentScope.PUBLIC
        else None,
    )


class Gateway:
    def __init__(self, count: int) -> None:
        self.count: int = count
        self.searches: int = 0
        self.reads: list[int] = []

    async def search_references(
        self, _context: AgentRequestContext, _query: str
    ) -> list[dict[str, object]]:
        self.searches += 1
        return [
            {
                "article_id": item,
                "title": f"Source {item}",
                "digest": "Metadata",
                "status": "published",
            }
            for item in range(1, self.count + 1)
        ]

    async def read_reference(
        self, _context: AgentRequestContext, article_id: int
    ) -> dict[str, object]:
        self.reads.append(article_id)
        return {
            "article_id": article_id,
            "title": f"Source {article_id}",
            "content": str(article_id) * 5000,
        }


class Generator:
    def __init__(self) -> None:
        self.inputs: list[PreparedInput] = []

    async def ainvoke(
        self, payload: object, _config: object, *, context: AgentRequestContext
    ) -> object:
        assert context.capability is Capability.WRITE
        message = GeneratorInput.model_validate(payload).messages[0].content
        data = PreparedInput.model_validate_json(message.split("\ninput=", 1)[1])
        self.inputs.append(data)
        ids = tuple(item.article_id for item in data.reference_fragments)
        return {
            "structured_response": ArticleDraft(
                title="Draft", outline="Outline", markdown="Body", source_article_ids=ids
            )
        }


def test_legacy_writing_digest_omits_new_default_fields() -> None:
    value = WritingInput(topic="Legacy")
    payload: dict[str, object] = {
        "topic": "Legacy",
        "requirements": "",
        "reference_article_ids": [],
    }
    encoded = json.dumps(payload, ensure_ascii=False, sort_keys=True, separators=(",", ":"))
    assert input_payload(value) == payload
    assert input_digest(value) == hashlib.sha256(encoded.encode()).hexdigest()
    assert "request_requirements" in input_payload(
        WritingInput(topic="Legacy", request_requirements=True)
    )


@pytest.mark.parametrize(
    "extra",
    [
        {"request_requirements": 1},
        {"request_requirements": True, "requirements": "Already filled"},
        {"reference_query": "query", "reference_article_ids": [1]},
    ],
)
def test_ambiguous_writing_input_rejected(extra: dict[str, object]) -> None:
    with pytest.raises(ValidationError):
        _ = WritingInput.model_validate({"topic": "Topic", **extra})


def request() -> SelectionRequest:
    return SelectionRequest(
        interaction_id="int_selection",
        title="Select",
        prompt="Select sources",
        options=tuple(
            SelectionOption(article_id=item, title="Source", digest="", status="published")
            for item in (1, 2)
        ),
        max_selections=2,
    )


@pytest.mark.parametrize(
    "payload",
    [
        None,
        {},
        {"article_ids": [1, 1]},
        {"article_ids": [3]},
        {"article_ids": [True]},
        {"article_ids": ["1"]},
        {"article_ids": [], "extra": 1},
    ],
)
def test_selection_rejects_invalid_business_payload(payload: object) -> None:
    with pytest.raises(ValueError, match="selected references|validation error"):
        _ = validate_interaction_response(
            request(), "select", payload_present=True, payload=payload
        )


def test_empty_selection_and_trimmed_requirements_are_real_responses() -> None:
    assert (
        validate_interaction_response(
            request(), "select", payload_present=True, payload={"article_ids": []}
        )
        == ()
    )
    text = TextInputRequest(
        interaction_id="int_text", title="Requirements", prompt="Add requirements"
    )
    assert (
        validate_interaction_response(
            text, "submit", payload_present=True, payload="  Concise prose  "
        )
        == "Concise prose"
    )
    for present, value in ((False, None), (True, None), (True, "  "), (True, "x" * 16001)):
        with pytest.raises(ValueError, match="payload is required|string|length is invalid"):
            _ = validate_interaction_response(
                text, "submit", payload_present=present, payload=value
            )


@pytest.mark.anyio
@pytest.mark.parametrize(("count", "selected"), [(0, []), (1, [1]), (2, []), (3, [2])])
async def test_production_preparation_checkpoints_and_reads_only_selected(
    count: int, selected: list[int]
) -> None:
    gateway = Gateway(count)
    generator = Generator()
    graph = prepared_writing_graph(generator, gateway, InMemorySaver())
    config = {"configurable": {"thread_id": "preparation"}}
    ctx = context()
    initial = WritingInput(topic="Topic", request_requirements=True, reference_query="sources")
    first = await graph.ainvoke(
        {"writing_input": input_payload(initial), "locale": "und"}, config, context=ctx
    )
    first_request = pending_interrupt(first)
    assert interruption_kind(first_request) == "text_input"
    assert gateway.searches == 0
    next_state = await graph.ainvoke(
        Command(resume={first_request.id: {"decision": "submit", "payload": "  Concise prose  "}}),
        config,
        context=ctx,
    )
    assert gateway.searches == 1
    if count >= 2:
        selection = pending_interrupt(next_state)
        assert interruption_kind(selection) == "selection"
        snapshot = await graph.aget_state(config)
        values = cast("Mapping[str, object]", snapshot.values)
        candidates = cast("list[dict[str, object]]", values["candidates"])
        assert all("content" not in item for item in candidates)
        next_state = await graph.ainvoke(
            Command(
                resume={selection.id: {"decision": "select", "payload": {"article_ids": selected}}}
            ),
            config,
            context=ctx,
        )
    confirmation = pending_interrupt(next_state)
    assert interruption_kind(confirmation) == "confirmation"
    assert gateway.searches == 1
    assert gateway.reads == selected
    assert len(generator.inputs) == 1
    actual = generator.inputs[0]
    assert actual.requirements == "Concise prose"
    assert [item.article_id for item in actual.reference_fragments] == selected
    assert all(
        len(item.fragment) == 4000 and item.is_fragment for item in actual.reference_fragments
    )
    final = await graph.ainvoke(
        Command(resume={confirmation.id: {"decision": "approve"}}), config, context=ctx
    )
    assert isinstance(final, Mapping)
    draft = cast("Mapping[str, object]", final)["structured_response"]
    assert isinstance(draft, ArticleDraft)
    assert draft.source_article_ids == tuple(selected)
    assert len(generator.inputs) == 1


@pytest.mark.anyio
@pytest.mark.parametrize("protocol", [ModelProtocol.CHAT_COMPLETIONS, ModelProtocol.RESPONSES])
async def test_public_middleware_sets_actual_sdk_protocol_output_limit(
    protocol: ModelProtocol,
) -> None:
    settings = policy_settings(protocol)
    middleware = PublicModelPolicy(settings)
    responses = protocol is ModelProtocol.RESPONSES
    model = PayloadChatOpenAI(
        model="public-test", api_key=SecretStr("test"), use_responses_api=responses
    )
    request_value = ModelRequest(
        model=model,
        messages=[HumanMessage(content="Search")],
        runtime=Runtime(context=context(ContentScope.PUBLIC)),
    )
    captured: list[object] = []

    async def handler(
        request: ModelRequest[AgentRequestContext],
    ) -> ModelResponse[object]:
        assert isinstance(request.model, PayloadChatOpenAI)
        payload = request.model.request_payload(request)
        assert payload["max_output_tokens" if responses else "max_completion_tokens"] == 2048
        assert request.model is model
        assert request.system_message is not None
        assert "published public" in request.system_message.content
        captured.append(payload)
        return ModelResponse[object](result=[AIMessage(content="bounded")])

    try:
        _ = await middleware.awrap_model_call(request_value, handler)
        assert len(captured) == 1
        assert model.max_tokens is None
    finally:
        await cast("AsyncClose", model.root_async_client).close()


@pytest.mark.anyio
async def test_public_middleware_rejects_unbounded_model_protocol() -> None:
    settings = policy_settings(ModelProtocol.RESPONSES)
    model = PayloadChatOpenAI(
        model="public-test", api_key=SecretStr("test"), use_responses_api=False
    )

    async def handler(
        _request: ModelRequest[AgentRequestContext],
    ) -> ModelResponse[object]:
        reason = "policy failure must precede provider dispatch"
        raise AssertionError(reason)

    try:
        request_value = ModelRequest(
            model=model, messages=[], runtime=Runtime(context=context(ContentScope.PUBLIC))
        )
        with pytest.raises(PolicyLimitError):
            _ = await PublicModelPolicy(settings).awrap_model_call(request_value, handler)
    finally:
        await cast("AsyncClose", model.root_async_client).close()


@pytest.mark.anyio
async def test_unselected_generated_source_fails_before_confirmation() -> None:
    class InventedGenerator(Generator):
        @override
        async def ainvoke(
            self, payload: object, config: object, *, context: AgentRequestContext
        ) -> object:
            _ = await super().ainvoke(payload, config, context=context)
            return {
                "structured_response": ArticleDraft(
                    title="Draft", outline="Outline", markdown="Body", source_article_ids=(99,)
                )
            }

    graph = prepared_writing_graph(
        InventedGenerator(),
        Gateway(1),
        InMemorySaver(),
    )
    value = WritingInput(topic="Topic", reference_query="sources")
    with pytest.raises(InvalidCapabilityOutputError):
        _ = await graph.ainvoke(
            {"writing_input": input_payload(value), "locale": "und"},
            {"configurable": {"thread_id": "invented"}},
            context=context(),
        )


@pytest.mark.anyio
async def test_selected_source_read_failure_never_generates_or_replaces_reference() -> None:
    class UnavailableGateway(Gateway):
        @override
        async def read_reference(
            self, _context: AgentRequestContext, article_id: int
        ) -> dict[str, object]:
            self.reads.append(article_id)
            reason = "selected article unavailable"
            raise OSError(reason)

    gateway = UnavailableGateway(2)
    generator = Generator()
    graph = prepared_writing_graph(generator, gateway, InMemorySaver())
    value = WritingInput(topic="Topic", reference_query="sources")
    config = {"configurable": {"thread_id": "unavailable"}}
    first = await graph.ainvoke(
        {"writing_input": input_payload(value), "locale": "und"}, config, context=context()
    )
    pending = pending_interrupt(first)
    with pytest.raises(OSError, match="selected article unavailable"):
        _ = await graph.ainvoke(
            Command(resume={pending.id: {"decision": "select", "payload": {"article_ids": [2]}}}),
            config,
            context=context(),
        )
    assert gateway.searches == 1
    assert gateway.reads == [2]
    assert generator.inputs == []
