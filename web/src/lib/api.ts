export const apiURL = process.env.NEXT_PUBLIC_API_URL ?? "http://localhost:8080";

export async function errorMessageFromResponse(response: Response): Promise<string | null> {
  try {
    const payload = (await response.json()) as { error?: unknown };
    if (typeof payload.error !== "string") {
      return null;
    }
    const message = payload.error.trim();
    if (!message) {
      return null;
    }
    return message.charAt(0).toUpperCase() + message.slice(1);
  } catch {
    return null;
  }
}

export function googleAuthURL(next?: string) {
  const url = new URL(`${apiURL}/api/v1/auth/google`);
  if (next && next.startsWith("/") && !next.startsWith("//")) {
    url.searchParams.set("next", next);
  }
  return url.toString();
}

export function newIdempotencyKey() {
  if (typeof crypto !== "undefined" && "randomUUID" in crypto) {
    return crypto.randomUUID();
  }
  return `order_${Date.now().toString(16)}_${Math.random().toString(16).slice(2)}`;
}
