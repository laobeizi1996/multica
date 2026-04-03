import { test, expect } from "@playwright/test";
import { loginAsDefault } from "./helpers";

test.describe("Settings", () => {
  test("updating workspace name reflects in sidebar immediately", async ({
    page,
  }) => {
    await loginAsDefault(page);

    const sidebarName = page.getByRole("button", { name: "Workspace menu" });
    const originalName = await sidebarName.innerText();

    await page.getByRole("link", { name: "Settings" }).click();
    await page.waitForURL("**/settings");
    await page.getByRole("tab", { name: "General" }).click();

    const nameInput = page.getByRole("textbox", { name: "Workspace name" });
    await nameInput.clear();
    const newName = "Renamed WS " + Date.now();
    await nameInput.fill(newName);

    await page.getByRole("button", { name: "Save" }).click();
    await expect(page.getByText("Workspace settings saved").first()).toBeVisible({ timeout: 5000 });
    await expect(sidebarName).toContainText(newName);

    await nameInput.clear();
    await nameInput.fill(originalName.trim());
    await page.getByRole("button", { name: "Save" }).click();
    await expect(page.getByText("Workspace settings saved").first()).toBeVisible({ timeout: 5000 });
  });
});
