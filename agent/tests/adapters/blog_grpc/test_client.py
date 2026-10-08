"""Exercise all eight BlogContent RPCs over a real local gRPC channel."""

from uuid import uuid4

import pytest

from scyg_agent.adapters.blog_grpc import (
    ArchiveArticle,
    ArticleId,
    ArticleSucceeded,
    ArticleTypesSucceeded,
    BlogGrpcClient,
    BlogUserId,
    CorrelationId,
    CreateArticle,
    GetArticle,
    ListArticleTypes,
    ListTags,
    PublishArticle,
    RequestId,
    RequestIdentity,
    SearchArticles,
    SearchSucceeded,
    TagId,
    TagsSucceeded,
    UpdateArticle,
)
from scyg_agent.domain.runs import OperationId, RunId, ToolCallId
from scyg_agent.generated.proto.scyg.blog.v1 import blog_content_service_pb2 as service_pb2
from scyg_agent.generated.proto.scyg.blog.v1 import common_pb2

from .support import FakeBlog, article, dispatch_call


def identity(suffix: str = "1") -> RequestIdentity:
    return RequestIdentity(
        RequestId(f"request-{suffix}"),
        CorrelationId("correlation-1"),
        RunId("run_abcdefgh"),
        ToolCallId(f"tool_abcdefg{suffix}"),
    )


@pytest.mark.anyio
@pytest.mark.parametrize(
    "status",
    [
        common_pb2.ARTICLE_STATUS_DRAFT,
        common_pb2.ARTICLE_STATUS_PUBLISHED,
        common_pb2.ARTICLE_STATUS_ARCHIVED,
    ],
)
async def test_get_maps_full_management_projection(
    blog_server: tuple[str, FakeBlog],
    status: common_pb2.ArticleStatus,
) -> None:
    target, fake = blog_server
    fake.article_status = status
    article_id = ArticleId(2**53 + 1)
    async with BlogGrpcClient(target, 1.0) as client:
        result = await client.invoke(
            "get_article",
            "v1",
            GetArticle(identity(), BlogUserId("admin"), article_id),
            dispatch=dispatch_call,
        )
    assert isinstance(result, ArticleSucceeded)
    assert result.article.article_id == article_id
    assert result.article.status == status
    assert result.article.content == "正文"
    assert result.article.digest == "摘要"
    assert result.article.article_type_id == 3
    assert result.article.slug == "article"
    assert result.article.version == 1
    assert result.article.tags[0].id == 2
    assert result.article.tags[0].name == "Tag"
    assert result.article.created_at == "1970-01-01T00:00:01Z"
    assert result.article.updated_at == "1970-01-01T00:00:02Z"
    assert (result.article.support, result.article.comment, result.article.visited) == (4, 5, 6)


@pytest.mark.anyio
async def test_search_maps_management_page(blog_server: tuple[str, FakeBlog]) -> None:
    target, _fake = blog_server
    type_id, tag_id = 2**53 + 3, 2**53 + 2
    command = SearchArticles(
        identity(),
        BlogUserId("admin"),
        "关键词",
        2,
        10,
        common_pb2.ARTICLE_STATUS_ARCHIVED,
        type_id,
        tag_id,
        "-updated_at",
    )
    async with BlogGrpcClient(target, 1.0) as client:
        result = await client.invoke("search_articles", "v1", command, dispatch=dispatch_call)
    assert isinstance(result, SearchSucceeded)
    assert len(result.articles) == 1
    assert (result.page, result.page_size, result.total_items, result.total_pages) == (2, 10, 11, 2)


@pytest.mark.anyio
async def test_search_receives_full_hundred_article_page_above_four_mib(
    blog_server: tuple[str, FakeBlog],
) -> None:
    target, fake = blog_server
    content = "x" * (50 * 1024)
    fake.search_articles = [article(index) for index in range(1, 101)]
    for item in fake.search_articles:
        item.content = content
    command = SearchArticles(identity(), BlogUserId("admin"), page_size=100)
    async with BlogGrpcClient(target, 5.0) as client:
        result = await client.invoke("search_articles", "v1", command, dispatch=dispatch_call)
    assert isinstance(result, SearchSucceeded)
    assert (result.page, result.page_size, result.total_items, result.total_pages) == (
        1,
        100,
        100,
        1,
    )
    assert [item.article_id for item in result.articles] == list(range(1, 101))
    assert all(item.content == content for item in result.articles)


@pytest.mark.anyio
async def test_search_defaults_use_database_pagination(blog_server: tuple[str, FakeBlog]) -> None:
    target, _fake = blog_server
    async with BlogGrpcClient(target, 1.0) as client:
        result = await client.invoke(
            "search_articles",
            "v1",
            SearchArticles(identity(), BlogUserId("admin")),
            dispatch=dispatch_call,
        )
    assert isinstance(result, SearchSucceeded)
    assert (result.page, result.page_size) == (1, 20)


@pytest.mark.anyio
async def test_taxonomy_rpc_queries_and_complete_pages(blog_server: tuple[str, FakeBlog]) -> None:
    """Both taxonomy consumers forward query/sort and map numeric resources."""
    target, _fake = blog_server
    async with BlogGrpcClient(target, 1.0) as client:
        tags = await client.invoke(
            "list_tags",
            "v1",
            ListTags(identity(), BlogUserId("admin"), 2, 10, "tag", "name"),
            dispatch=dispatch_call,
        )
        types = await client.invoke(
            "list_article_types",
            "v1",
            ListArticleTypes(
                identity(),
                BlogUserId("admin"),
                3,
                5,
                "type",
                "-name",
            ),
            dispatch=dispatch_call,
        )
    assert isinstance(tags, TagsSucceeded)
    assert (tags.tags[0].id, tags.tags[0].name) == (2, "Tag")
    assert (tags.page, tags.page_size, tags.total_items, tags.total_pages) == (2, 10, 1, 1)
    assert isinstance(types, ArticleTypesSucceeded)
    assert (types.article_types[0].id, types.article_types[0].name) == (3, "Type")
    assert (types.page, types.page_size, types.total_items, types.total_pages) == (3, 5, 1, 1)


@pytest.mark.anyio
async def test_four_write_rpcs_forward_every_field_and_stable_uuid_retries(
    blog_server: tuple[str, FakeBlog],
) -> None:
    """Each new intent has its own UUID; retries transmit the original identity."""
    target, fake = blog_server
    user = BlogUserId("admin")
    article_id, type_id, tag_id = ArticleId(2**53 + 1), 2**53 + 3, TagId(2**53 + 2)
    commands = (
        (
            "create_article",
            CreateArticle(
                identity(),
                user,
                OperationId(str(uuid4())),
                common_pb2.ARTICLE_STATUS_DRAFT,
                type_id,
                "Title",
                "slug",
                "digest",
                "content",
                (tag_id,),
            ),
        ),
        (
            "update_article",
            UpdateArticle(
                identity(),
                user,
                OperationId(str(uuid4())),
                article_id,
                7,
                type_id,
                "Title",
                "slug",
                "digest",
                "content",
                (tag_id,),
            ),
        ),
        (
            "publish_article",
            PublishArticle(identity(), user, OperationId(str(uuid4())), article_id, 8),
        ),
        (
            "archive_article",
            ArchiveArticle(identity(), user, OperationId(str(uuid4())), article_id, 9),
        ),
    )
    async with BlogGrpcClient(target, 1.0) as client:
        for name, command in commands:
            for _ in range(2):
                result = await client.invoke(name, "v1", command, dispatch=dispatch_call)
                assert isinstance(result, ArticleSucceeded)
                assert result.article.article_id == (1 if name == "create_article" else article_id)
                if name == "publish_article":
                    assert result.article.status == common_pb2.ARTICLE_STATUS_PUBLISHED
                if name == "archive_article":
                    assert result.article.status == common_pb2.ARTICLE_STATUS_ARCHIVED
    assert fake.calls == [
        "create",
        "create",
        "update",
        "update",
        "publish",
        "publish",
        "archive",
        "archive",
    ]
    assert len(set(fake.operation_ids)) == 4
    assert all(
        (fake.operation_ids[index] == fake.operation_ids[index + 1] for index in range(0, 8, 2)),
    )
    create, update, publish, archive = (fake.requests[index] for index in (0, 2, 4, 6))
    assert isinstance(create, service_pb2.CreateArticleRequest)
    assert isinstance(update, service_pb2.UpdateArticleRequest)
    assert isinstance(publish, service_pb2.PublishArticleRequest)
    assert isinstance(archive, service_pb2.ArchiveArticleRequest)
    assert (
        create.user_id,
        create.status,
        create.article_type_id,
        create.title,
        create.slug,
        create.digest,
        create.content,
        list(create.tag_ids),
    ) == ("admin", 1, type_id, "Title", "slug", "digest", "content", [tag_id])
    assert (
        update.user_id,
        update.article_id,
        update.expected_version,
        update.article_type_id,
        update.title,
        update.slug,
        update.digest,
        update.content,
        list(update.tag_ids.values),
    ) == ("admin", article_id, 7, type_id, "Title", "slug", "digest", "content", [tag_id])
    assert all(
        (
            update.HasField(field)
            for field in (
                "article_type_id",
                "title",
                "slug",
                "digest",
                "content",
                "tag_ids",
            )
        ),
    )
    assert (
        publish.user_id,
        publish.article_id,
        publish.expected_version,
    ) == ("admin", article_id, 8)
    assert (
        archive.user_id,
        archive.article_id,
        archive.expected_version,
    ) == ("admin", article_id, 9)


@pytest.mark.anyio
async def test_update_absent_fields_differ_from_explicit_empty(
    blog_server: tuple[str, FakeBlog],
) -> None:
    """A sparse patch and an empty replacement have distinct wire presence."""
    target, fake = blog_server
    async with BlogGrpcClient(target, 1.0) as client:
        sparse = UpdateArticle(
            identity(),
            BlogUserId("admin"),
            OperationId(str(uuid4())),
            ArticleId(1),
            1,
        )
        clear = UpdateArticle(
            identity(),
            BlogUserId("admin"),
            OperationId(str(uuid4())),
            ArticleId(1),
            1,
            title="",
            slug="",
            digest="",
            content="",
            tag_ids=(),
        )
        for command in (sparse, clear):
            assert isinstance(
                await client.invoke("update_article", "v1", command, dispatch=dispatch_call),
                ArticleSucceeded,
            )
    sparse_request, clear_request = fake.requests
    assert isinstance(sparse_request, service_pb2.UpdateArticleRequest)
    assert isinstance(clear_request, service_pb2.UpdateArticleRequest)
    assert all(
        (
            not sparse_request.HasField(field)
            for field in (
                "article_type_id",
                "title",
                "slug",
                "digest",
                "content",
                "tag_ids",
            )
        ),
    )
    assert all(
        (
            clear_request.HasField(field)
            for field in (
                "title",
                "slug",
                "digest",
                "content",
                "tag_ids",
            )
        ),
    )
    assert not clear_request.HasField("article_type_id")
    assert list(clear_request.tag_ids.values) == []
