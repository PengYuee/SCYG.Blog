import { defineConfig } from "@hey-api/openapi-ts"

export default defineConfig({
  input: "../backend/api/openapi.yaml",
  output: "src/request/generated",
  plugins: [
    "@hey-api/typescript",
    {
      name: "zod",
      definitions: true,
      requests: false,
      responses: false,
      $resolvers: {
        /** 保持现有 JSON number 领域契约，并由 Zod int 拒绝不安全整数。 */
        number(ctx) {
          if (ctx.schema.format !== "int64") return
          const { $, schema, symbols } = ctx
          let chain = $(symbols.z).attr("number").call().attr("int").call()
          if (schema.minimum !== undefined) chain = chain.attr("gte").call($.literal(schema.minimum))
          if (schema.exclusiveMinimum !== undefined) chain = chain.attr("gt").call($.literal(schema.exclusiveMinimum))
          if (schema.maximum !== undefined) chain = chain.attr("lte").call($.literal(schema.maximum))
          if (schema.exclusiveMaximum !== undefined) chain = chain.attr("lt").call($.literal(schema.exclusiveMaximum))
          return chain
        },
        /** 所有带属性的 OpenAPI 对象均声明 additionalProperties: false。 */
        object(ctx) {
          if (ctx.schema.properties === undefined || Object.keys(ctx.schema.properties).length === 0) return
          const { $, symbols } = ctx
          const shape = ctx.nodes.shape(ctx)
          ctx.nodes.base = () => $(symbols.z).attr("strictObject").call(shape)
        },
      },
    },
  ],
})
