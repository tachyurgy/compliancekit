import { useCallback, useEffect, useState } from "react";
import { useParams } from "react-router-dom";
import { PdfSigner } from "../components/PdfSigner/PdfSigner";
import { finalizeSignature, prepareSignSession } from "../components/PdfSigner/api";
import type { AuditRecord, SignSession } from "../components/PdfSigner/types";

export default function SignDocument() {
  const { token } = useParams<{ token: string }>();
  const [session, setSession] = useState<SignSession | null>(null);
  const [loadError, setLoadError] = useState<string | null>(null);
  const [completed, setCompleted] = useState<{
    signatureId: string;
    sha256After: string;
  } | null>(null);

  useEffect(() => {
    if (!token) return;
    let cancelled = false;
    (async () => {
      try {
        const s = await prepareSignSession(token);
        if (!cancelled) setSession(s);
      } catch (err) {
        if (!cancelled) setLoadError((err as Error).message);
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [token]);

  const handleComplete = useCallback(
    async (args: { signedBlob: Blob; auditRecord: AuditRecord }) => {
      if (!token) return;
      const rec = await finalizeSignature(token, args.signedBlob, args.auditRecord);
      setCompleted({ signatureId: rec.signatureId, sha256After: rec.sha256After });
    },
    [token],
  );

  if (!token) {
    return <div style={shellStyle}>Missing signing token.</div>;
  }
  if (loadError) {
    return (
      <div style={shellStyle}>
        <h2>Unable to load signing session</h2>
        <p>{loadError}</p>
        <p>Your link may have expired. Please contact the sender for a new invitation.</p>
      </div>
    );
  }
  if (!session) {
    return <div style={shellStyle}>Loading your document\u2026</div>;
  }
  if (completed) {
    return (
      <div style={shellStyle}>
        <h2>Signature complete</h2>
        <p>
          Thank you, {session.signerName}. Your signed document has been recorded.
        </p>
        <dl>
          <dt>Signature ID</dt>
          <dd>
            <code>{completed.signatureId}</code>
          </dd>
          <dt>Document hash (SHA-256)</dt>
          <dd>
            <code style={{ wordBreak: "break-all" }}>{completed.sha256After}</code>
          </dd>
        </dl>
        <p style={{ fontSize: 13, color: "#57606a" }}>
          A copy has been emailed to {session.signerEmail}. Keep this signature
          ID for your records.
        </p>
      </div>
    );
  }

  return (
    <div style={shellStyle}>
      <header style={{ marginBottom: 20 }}>
        <h1 style={{ margin: 0, fontSize: 22 }}>Sign document</h1>
        <div style={{ color: "#57606a", fontSize: 14 }}>
          Signer: {session.signerName} &lt;{session.signerEmail}&gt; \u00b7 Session expires{" "}
          {new Date(session.expiresAt).toLocaleString()}
        </div>
      </header>
      <PdfSigner session={session} onComplete={handleComplete} />
    </div>
  );
}

const shellStyle: React.CSSProperties = {
  maxWidth: 900,
  margin: "0 auto",
  padding: "24px 16px 64px",
  fontFamily: "-apple-system, BlinkMacSystemFont, 'Segoe UI', sans-serif",
};
