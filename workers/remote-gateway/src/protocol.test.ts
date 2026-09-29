import { describe, expect, it } from "vitest";
import { controllerMessage, offeredProtocols, parseDeviceMessage } from "./protocol";

describe("remote message routing envelope", () => {
  it("carries connection scopes outside the opaque payload", () => {
    expect(JSON.parse(controllerMessage("a".repeat(32), ["terminal"], "encrypted-body"))).toEqual({
      type: "controller_message",
      connectionId: "a".repeat(32),
      scopes: ["terminal"],
      payload: "encrypted-body",
    });
  });

  it("accepts only a directed device reply", () => {
    expect(parseDeviceMessage(JSON.stringify({ to: "b".repeat(32), payload: "encrypted-response" }))).toEqual({
      type: "reply",
      to: "b".repeat(32),
      payload: "encrypted-response",
    });
    expect(parseDeviceMessage(JSON.stringify({ payload: "broadcast" }))).toBeNull();
    expect(parseDeviceMessage("not-json")).toBeNull();
  });

  it("accepts a device heartbeat", () => {
    expect(parseDeviceMessage(JSON.stringify({ type: "heartbeat" }))).toEqual({ type: "heartbeat" });
  });

  it("accepts a device-owned controller disconnect", () => {
    expect(parseDeviceMessage(JSON.stringify({
      type: "disconnect_controller",
      connectionId: "c".repeat(32),
    }))).toEqual({ type: "disconnect_controller", connectionId: "c".repeat(32) });
    expect(parseDeviceMessage(JSON.stringify({
      type: "disconnect_controller",
      connectionId: "not-a-connection",
    }))).toBeNull();
  });
});

describe("WebSocket protocol offers", () => {
  it("parses the browser's ordered protocol list", () => {
    const request = new Request("https://remote.reasonix.io/v1/sessions/connect", {
      headers: { "sec-websocket-protocol": "reasonix.remote.v1, reasonix.auth.ticket" },
    });
    expect(offeredProtocols(request)).toEqual(["reasonix.remote.v1", "reasonix.auth.ticket"]);
  });
});
