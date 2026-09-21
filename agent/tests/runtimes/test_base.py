"""运行时适配器纯应用协议测试。"""

from collections.abc import AsyncIterator
from typing import get_type_hints

from scyg_agent.domain.runs import Run
from scyg_agent.runtimes.base import RuntimeAdapter
from scyg_agent.runtimes.outputs import RuntimeNativeOutput


def test_runtime_adapter_exposes_typed_application_stream_signatures() -> None:
    # Given: T14 的协议声明, 不加载任何框架适配器.
    execute_hints = get_type_hints(RuntimeAdapter.execute)
    resume_hints = get_type_hints(RuntimeAdapter.resume)

    # When/Then: 执行与恢复仅接收 Run 并返回应用原生结果异步流。
    assert execute_hints["run"] is Run
    assert execute_hints["return"] == AsyncIterator[RuntimeNativeOutput]
    assert resume_hints["run"] is Run
    assert resume_hints["return"] == AsyncIterator[RuntimeNativeOutput]
