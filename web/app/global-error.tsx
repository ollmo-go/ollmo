"use client";

export default function GlobalError({
  error,
  reset,
}: {
  error: Error & { digest?: string };
  reset: () => void;
}) {
  return (
    <html lang="en">
      <body style={{ margin: 0, fontFamily: "system-ui, sans-serif" }}>
        <div style={{ display: "flex", minHeight: "100vh", flexDirection: "column", alignItems: "center", justifyContent: "center", gap: "16px", textAlign: "center", padding: "24px" }}>
          <h2 style={{ fontSize: "18px", fontWeight: 600 }}>Something went wrong</h2>
          <p style={{ fontSize: "14px", color: "#666", maxWidth: "400px" }}>
            {error.message || "An unexpected error occurred."}
          </p>
          <button
            onClick={reset}
            style={{ padding: "8px 16px", borderRadius: "6px", border: "1px solid #ccc", background: "transparent", cursor: "pointer", fontSize: "14px" }}
          >
            Try again
          </button>
        </div>
      </body>
    </html>
  );
}
