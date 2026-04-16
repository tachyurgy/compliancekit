import { useEffect, useState } from "react";
import { FieldDesigner } from "../components/PdfSigner/FieldDesigner";
import { listTemplates, type TemplateSummary } from "../components/PdfSigner/api";

const API_BASE =
  (import.meta as unknown as { env?: { VITE_API_BASE?: string } }).env?.VITE_API_BASE ?? "";

export default function DocumentTemplates() {
  const [templates, setTemplates] = useState<TemplateSummary[] | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [editing, setEditing] = useState<TemplateSummary | null>(null);

  useEffect(() => {
    (async () => {
      try {
        setTemplates(await listTemplates());
      } catch (err) {
        setError((err as Error).message);
      }
    })();
  }, []);

  if (editing) {
    return (
      <div style={shellStyle}>
        <button
          type="button"
          onClick={() => setEditing(null)}
          style={{ marginBottom: 12 }}
        >
          \u2190 Back to templates
        </button>
        <FieldDesigner
          templateId={editing.id}
          templateName={editing.name}
          documentUrl={`${API_BASE}/api/pdfsign/templates/${editing.id}/pdf`}
          onSaved={() => setEditing(null)}
        />
      </div>
    );
  }

  return (
    <div style={shellStyle}>
      <header
        style={{
          display: "flex",
          alignItems: "center",
          justifyContent: "space-between",
          marginBottom: 20,
        }}
      >
        <h1 style={{ margin: 0, fontSize: 22 }}>Document templates</h1>
        <button
          type="button"
          onClick={() => alert("Upload flow lives elsewhere; wire here.")}
          style={{
            background: "#1f883d",
            color: "#fff",
            border: "none",
            padding: "8px 14px",
            borderRadius: 4,
            cursor: "pointer",
          }}
        >
          Upload PDF
        </button>
      </header>

      {error && <div role="alert" style={{ color: "#cf222e" }}>{error}</div>}

      {!templates ? (
        <div>Loading\u2026</div>
      ) : templates.length === 0 ? (
        <div style={{ color: "#57606a" }}>
          No templates yet. Upload a PDF to drag signature fields onto it.
        </div>
      ) : (
        <table style={{ width: "100%", borderCollapse: "collapse" }}>
          <thead>
            <tr style={{ textAlign: "left", borderBottom: "1px solid #d0d7de" }}>
              <th style={cellStyle}>Name</th>
              <th style={cellStyle}>Pages</th>
              <th style={cellStyle}>Fields</th>
              <th style={cellStyle}>Last updated</th>
              <th style={cellStyle}></th>
            </tr>
          </thead>
          <tbody>
            {templates.map((t) => (
              <tr key={t.id} style={{ borderBottom: "1px solid #eaeef2" }}>
                <td style={cellStyle}>{t.name}</td>
                <td style={cellStyle}>{t.pageCount}</td>
                <td style={cellStyle}>{t.fieldCount}</td>
                <td style={cellStyle}>
                  {new Date(t.updatedAt).toLocaleDateString()}
                </td>
                <td style={cellStyle}>
                  <button type="button" onClick={() => setEditing(t)}>
                    Edit fields
                  </button>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </div>
  );
}

const shellStyle: React.CSSProperties = {
  maxWidth: 1200,
  margin: "0 auto",
  padding: "24px 16px 64px",
  fontFamily: "-apple-system, BlinkMacSystemFont, 'Segoe UI', sans-serif",
};

const cellStyle: React.CSSProperties = { padding: "10px 8px" };
