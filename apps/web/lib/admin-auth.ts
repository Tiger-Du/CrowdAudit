import "server-only";
import { cookies } from "next/headers";

export async function requireAdmin() {
  const token = process.env.ADMIN_TOKEN;
  if (!token) {
    throw new Error("ADMIN_TOKEN is not set");
  }

  const cookieStore = await cookies();
  const cookieToken = cookieStore.get("crowdaudit_admin")?.value;

  if (cookieToken !== token) {
    // Throwing gives you a hard fail; you can render a login page if preferred.
    throw new Error("Unauthorized (missing or invalid admin token)");
  }
}
