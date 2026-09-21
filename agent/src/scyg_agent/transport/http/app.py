"""创建带中文安全校验错误的 T20 ASGI 应用。."""

from fastapi import FastAPI, Request, status
from fastapi.exceptions import RequestValidationError
from fastapi.responses import JSONResponse

from .router import HTTPDependencies, create_http_router
from .schemas import ErrorResponse


def create_http_app(dependencies: HTTPDependencies) -> FastAPI:
    """组合固定 HTTP 路由并隐藏 Pydantic 输入诊断。."""
    app = FastAPI(title="SCYG Agent HTTP")

    @app.exception_handler(RequestValidationError)
    async def validation_error(_request: Request, _error: RequestValidationError) -> JSONResponse:
        """返回不包含原始输入或内部校验细节的中文错误。."""
        payload = ErrorResponse(detail="请求参数无效")
        return JSONResponse(
            status_code=status.HTTP_422_UNPROCESSABLE_CONTENT,
            content=payload.model_dump(),
        )

    _ = validation_error
    app.include_router(create_http_router(dependencies))
    return app
