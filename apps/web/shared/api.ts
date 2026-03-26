import { ApiClient } from "@multica/sdk";
import { createLogger } from "./logger";

const API_BASE_URL = process.env.NEXT_PUBLIC_API_URL ?? "http://localhost:8080";

export const api = new ApiClient(API_BASE_URL, { logger: createLogger("api") });

// Initialize token from localStorage on load
if (typeof window !== "undefined") {
  const token = localStorage.getItem("multica_token");
  if (token) {
    api.setToken(token);
  }
  const wsId = localStorage.getItem("multica_workspace_id");
  if (wsId) {
    api.setWorkspaceId(wsId);
  }

  api.setOnUnauthorized(() => {
    localStorage.removeItem("multica_token");
    localStorage.removeItem("multica_workspace_id");
    api.setToken(null);
    api.setWorkspaceId(null);
    if (window.location.pathname !== "/login") {
      window.location.href = "/login";
    }
  });
}
