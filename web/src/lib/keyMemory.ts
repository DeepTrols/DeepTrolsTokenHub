// In-memory, non-persistent store for freshly revealed API key plaintexts.
//
// Secrets must NEVER touch localStorage/sessionStorage — a single XSS would
// exfiltrate every stored key. Module-level state is lost on page refresh,
// which is the intended trade-off: a key is revealed once, copied by the
// user, and never written to disk.

const secrets = new Map<string, string>();

// One-time cleanup of legacy plaintext that older versions persisted to
// localStorage (before this fix). Safe no-op if already removed.
try {
  localStorage.removeItem("api_key_secrets");
} catch {
  /* ignore (non-browser or storage unavailable) */
}

export function setKeySecret(id: string, plaintext: string): void {
  secrets.set(id, plaintext);
}

export function getKeySecret(id: string): string | undefined {
  return secrets.get(id);
}

export function clearKeySecrets(): void {
  secrets.clear();
}
