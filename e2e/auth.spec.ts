import { test, expect } from "@playwright/test";
import { loginAsDefault, openWorkspaceMenu } from "./helpers";

test.describe("Authentication", () => {
  test("login page renders correctly", async ({ page }) => {
    await page.goto("/login");

    await expect(page.getByText("Multica")).toBeVisible();
    await expect(page.getByRole("textbox", { name: "Email" })).toBeVisible();
    await expect(page.getByRole("button", { name: "Continue" })).toBeVisible();
  });

  test("login and redirect to /issues", async ({ page }) => {
    await loginAsDefault(page);

    await expect(page).toHaveURL(/\/issues/);
    await expect(page.getByRole("button", { name: "All" })).toBeVisible();
    await expect(page.getByText("Backlog")).toBeVisible();
  });

  test("unauthenticated user is redirected to /login", async ({ page }) => {
    await page.goto("/login");
    await page.evaluate(() => {
      localStorage.removeItem("multica_token");
      localStorage.removeItem("multica_workspace_id");
    });

    await page.goto("/issues");
    await page.waitForURL("**/", { timeout: 10000 });
  });

  test("logout redirects to /", async ({ page }) => {
    await loginAsDefault(page);

    await openWorkspaceMenu(page);
    await page.getByRole("menuitem", { name: "Log out" }).click();

    await page.waitForURL("**/", { timeout: 10000 });
    await expect(page).toHaveURL(/\/$/);
  });
});
