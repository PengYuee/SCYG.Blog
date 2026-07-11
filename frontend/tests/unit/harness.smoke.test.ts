import { describe, expect, it } from "vitest"
import { parseT2Fixture, resolveT2Route } from "../fixtures/t2-harness"

describe("T2 unit harness", () => {
  it("reports the named smoke state when the harness is ready", () => {
    // Given: the isolated T2 smoke route exists.
    const route = "/t2-smoke"

    // When: the harness resolves that route.
    const state = resolveT2Route(route)

    // Then: the named route reports the ready state after the harness correction.
    expect(state).toBe("ready")
  })

  it("reports malformed input through the named T2 parser fixture", () => {
    // Given: the input lacks the required title field.
    const malformedInput: unknown = { heading: "wrong field" }

    // When: the isolated fixture parses the boundary value.
    const result = parseT2Fixture(malformedInput)

    // Then: malformed input is represented as a named binary result.
    expect(result.kind).toBe("malformed")
  })

  it("reports a missing route through the named T2 route fixture", () => {
    // Given: the route is intentionally absent from the fixture table.
    const missingRoute = "/t2-missing"

    // When: the isolated fixture resolves the path.
    const state = resolveT2Route(missingRoute)

    // Then: the missing route produces the explicit failure state.
    expect(state).toBe("missing")
  })
})

