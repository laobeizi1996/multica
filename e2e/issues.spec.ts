import { test, expect } from "@playwright/test";
import { loginAsDefault, createTestApi } from "./helpers";
import type { TestApiClient } from "./fixtures";

test.describe("Issues", () => {
  let api: TestApiClient;

  test.beforeEach(async ({ page }) => {
    api = await createTestApi();
    await loginAsDefault(page);
  });

  test.afterEach(async () => {
    await api.cleanup();
  });

  test("issues page loads with board view", async ({ page }) => {
    await expect(page.getByRole("button", { name: "All" })).toBeVisible();

    await expect(page.getByText("Backlog")).toBeVisible();
    await expect(page.getByText("Todo")).toBeVisible();
    await expect(page.getByText("In Progress")).toBeVisible();
    await expect(page.getByRole("button", { name: "Add issue to Backlog" })).toBeVisible();
  });

  test("can switch between board and list view", async ({ page }) => {
    await expect(page.getByRole("button", { name: "View mode" })).toBeVisible();
    await expect(page.getByRole("button", { name: "Add issue to Backlog" })).toBeVisible();

    await page.getByRole("button", { name: "View mode" }).click();
    await page.getByRole("menuitem", { name: "List" }).click();
    await expect(page.getByRole("button", { name: "Add issue to Backlog" })).not.toBeVisible();
    await expect(page.getByText("Backlog")).toBeVisible();

    await page.getByRole("button", { name: "View mode" }).click();
    await page.getByRole("menuitem", { name: "Board" }).click();
    await expect(page.getByRole("button", { name: "Add issue to Backlog" })).toBeVisible();
  });

  test("can create a new issue", async ({ page }) => {
    await page.getByRole("button", { name: "New issue" }).click();

    const title = "E2E Created " + Date.now();
    await page.getByRole("textbox", { name: "Issue title" }).fill(title);
    await page.getByRole("button", { name: "Create Issue" }).click();

    // New issue should appear on the page
    await expect(page.locator(`text=${title}`).first()).toBeVisible({
      timeout: 10000,
    });
  });

  test("can navigate to issue detail page", async ({ page }) => {
    // Create a known issue via API so the test controls its own fixture
    const issue = await api.createIssue("E2E Detail Test " + Date.now());

    // Reload to see the new issue
    await page.reload();
    await expect(page.getByRole("button", { name: "All" })).toBeVisible();

    // Navigate to the issue detail
    const issueLink = page.locator(`a[href="/issues/${issue.id}"]`);
    await expect(issueLink).toBeVisible({ timeout: 5000 });
    await issueLink.click();

    await page.waitForURL(/\/issues\/[\w-]+/);

    // Should show Properties panel
    await expect(page.locator("text=Properties")).toBeVisible();
    // Should show breadcrumb link back to Issues
    await expect(
      page.locator("a", { hasText: "Issues" }).first(),
    ).toBeVisible();
  });

  test("can cancel issue creation", async ({ page }) => {
    await page.getByRole("button", { name: "New issue" }).click();

    await expect(page.getByRole("textbox", { name: "Issue title" })).toBeVisible();

    await page.getByRole("button", { name: "Close issue modal" }).click();

    await expect(page.getByRole("textbox", { name: "Issue title" })).not.toBeVisible();
    await expect(page.getByRole("button", { name: "New issue" })).toBeVisible();
  });
});
