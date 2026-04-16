import { useCallback, useEffect, useMemo, useState } from "react";
import { Document, Page, pdfjs } from "react-pdf";
import { PDFDocument } from "pdf-lib";
import { FieldOverlay } from "./FieldOverlay";
import type { Field, FieldType } from "./types";
import { saveTemplateFields } from "./api";

pdfjs.GlobalWorkerOptions.workerSrc = "/pdf.worker.min.mjs";

export interface FieldDesignerProps {
  templateId: string;
  templateName: string;
  documentUrl: string;
  initialFields?: Field[];
  onSaved?: (fields: Field[]) => void;
}

const DEFAULT_SIZE: Record<FieldType, { w: number; h: number }> = {
  signature: { w: 180, h: 48 },
  initial: { w: 72, h: 32 },
  date: { w: 110, h: 20 },
  text: { w: 180, h: 22 },
  checkbox: { w: 16, h: 16 },
};

export function FieldDesigner({
  templateId,
  templateName,
  documentUrl,
  initialFields,
  onSaved,
}: FieldDesignerProps) {
  const [pdfBytes, setPdfBytes] = useState<Uint8Array | null>(null);
  const [pageCount, setPageCount] = useState(0);
  const [pageSizes, setPageSizes] = useState<Array<{ w: number; h: number }>>([]);
  const [renderedWidth] = useState(816);
  const [fields, setFields] = useState<Field[]>(initialFields ?? []);
  const [selectedId, setSelectedId] = useState<string | null>(null);
  const [paletteType, setPaletteType] = useState<FieldType>("signature");
  const [saving, setSaving] = useState(false);
  const [saveError, setSaveError] = useState<string | null>(null);

  useEffect(() => {
    let cancelled = false;
    (async () => {
      const resp = await fetch(documentUrl, { credentials: "include" });
      const buf = new Uint8Array(await resp.arrayBuffer());
      if (!cancelled) setPdfBytes(buf);
    })();
    return () => {
      cancelled = true;
    };
  }, [documentUrl]);

  const fileForReactPdf = useMemo(
    () => (pdfBytes ? { data: new Uint8Array(pdfBytes) } : null),
    [pdfBytes],
  );

  const onDocumentLoad = useCallback(
    async ({ numPages }: { numPages: number }) => {
      setPageCount(numPages);
      if (!pdfBytes) return;
      const doc = await PDFDocument.load(pdfBytes);
      setPageSizes(
        doc.getPages().map((p) => ({ w: p.getSize().width, h: p.getSize().height })),
      );
    },
    [pdfBytes],
  );

  const addFieldAt = useCallback(
    (pageIndex: number, evt: React.MouseEvent<HTMLDivElement>, ps: { w: number; h: number }) => {
      const rect = evt.currentTarget.getBoundingClientRect();
      const scaleX = rect.width / ps.w;
      const scaleY = rect.height / ps.h;
      const clickX = (evt.clientX - rect.left) / scaleX;
      const clickY = ps.h - (evt.clientY - rect.top) / scaleY;
      const size = DEFAULT_SIZE[paletteType];
      const newField: Field = {
        id: genId(),
        type: paletteType,
        pageIndex,
        x: Math.max(0, clickX - size.w / 2),
        y: Math.max(0, clickY - size.h / 2),
        width: size.w,
        height: size.h,
        required: true,
      };
      setFields((prev) => [...prev, newField]);
      setSelectedId(newField.id);
    },
    [paletteType],
  );

  const updateField = useCallback(
    (id: string, patch: Partial<Field>) => {
      setFields((prev) => prev.map((f) => (f.id === id ? { ...f, ...patch } : f)));
    },
    [],
  );

  const deleteField = useCallback((id: string) => {
    setFields((prev) => prev.filter((f) => f.id !== id));
    setSelectedId(null);
  }, []);

  const handleSave = useCallback(async () => {
    setSaving(true);
    setSaveError(null);
    try {
      await saveTemplateFields(
        templateId,
        fields.map((f) => ({
          type: f.type,
          pageIndex: f.pageIndex,
          x: f.x,
          y: f.y,
          width: f.width,
          height: f.height,
          required: f.required,
          label: f.label,
        })),
      );
      onSaved?.(fields);
    } catch (err) {
      setSaveError((err as Error).message);
    } finally {
      setSaving(false);
    }
  }, [fields, templateId, onSaved]);

  if (!fileForReactPdf) {
    return <div>Loading template PDF\u2026</div>;
  }

  return (
    <div style={{ display: "flex", gap: 16, alignItems: "flex-start" }}>
      <aside
        style={{
          position: "sticky",
          top: 16,
          minWidth: 220,
          border: "1px solid #d0d7de",
          borderRadius: 8,
          padding: 12,
          background: "#f6f8fa",
        }}
      >
        <h3 style={{ marginTop: 0 }}>{templateName}</h3>
        <div style={{ fontSize: 12, color: "#57606a", marginBottom: 10 }}>
          Click a palette button, then click on the page where the field should go.
        </div>
        <div style={{ display: "grid", gap: 6 }}>
          {(["signature", "initial", "date", "text", "checkbox"] as FieldType[]).map((t) => (
            <button
              key={t}
              type="button"
              onClick={() => setPaletteType(t)}
              style={{
                textAlign: "left",
                padding: "8px 10px",
                background: paletteType === t ? "#0969da" : "#fff",
                color: paletteType === t ? "#fff" : "#24292f",
                border: "1px solid #d0d7de",
                borderRadius: 4,
                cursor: "pointer",
              }}
            >
              {t}
            </button>
          ))}
        </div>
        <hr style={{ margin: "12px 0", borderColor: "#d0d7de" }} />
        <div style={{ fontSize: 12 }}>{fields.length} fields total</div>
        <button
          type="button"
          onClick={handleSave}
          disabled={saving}
          style={{
            marginTop: 8,
            width: "100%",
            padding: "8px 10px",
            background: "#1f883d",
            color: "#fff",
            border: "none",
            borderRadius: 4,
            cursor: saving ? "wait" : "pointer",
          }}
        >
          {saving ? "Saving\u2026" : "Save template"}
        </button>
        {saveError && (
          <div role="alert" style={{ color: "#cf222e", marginTop: 8, fontSize: 12 }}>
            {saveError}
          </div>
        )}
        {selectedId && (
          <FieldInspector
            field={fields.find((f) => f.id === selectedId)!}
            onChange={(patch) => updateField(selectedId, patch)}
            onDelete={() => deleteField(selectedId)}
          />
        )}
      </aside>

      <div style={{ flex: 1 }}>
        <Document file={fileForReactPdf} onLoadSuccess={onDocumentLoad}>
          {Array.from({ length: pageCount }, (_, idx) => {
            const ps = pageSizes[idx];
            if (!ps) return null;
            const pageFields = fields.filter((f) => f.pageIndex === idx);
            const rh = (renderedWidth * ps.h) / ps.w;
            return (
              <div
                key={idx}
                style={{ position: "relative", margin: "0 auto 16px", width: renderedWidth }}
                onClick={(e) => {
                  // Only add when clicking the page surface itself, not existing fields.
                  if (e.target === e.currentTarget) addFieldAt(idx, e, ps);
                }}
              >
                <Page
                  pageNumber={idx + 1}
                  width={renderedWidth}
                  renderAnnotationLayer={false}
                  renderTextLayer={false}
                />
                <FieldOverlay
                  fields={pageFields}
                  renderedWidth={renderedWidth}
                  renderedHeight={rh}
                  pageWidth={ps.w}
                  pageHeight={ps.h}
                  filledFieldIds={new Set()}
                  editable
                  selectedFieldId={selectedId ?? undefined}
                  onSelectField={setSelectedId}
                  onFieldMove={(id, next) => updateField(id, next)}
                  onFieldResize={(id, next) => updateField(id, next)}
                  onFieldDelete={deleteField}
                />
              </div>
            );
          })}
        </Document>
      </div>
    </div>
  );
}

function FieldInspector({
  field,
  onChange,
  onDelete,
}: {
  field: Field;
  onChange: (patch: Partial<Field>) => void;
  onDelete: () => void;
}) {
  return (
    <div style={{ marginTop: 12, fontSize: 12 }}>
      <hr style={{ borderColor: "#d0d7de" }} />
      <div style={{ fontWeight: 600, marginBottom: 6 }}>Selected field</div>
      <label style={{ display: "block", marginBottom: 4 }}>
        Label
        <input
          type="text"
          value={field.label ?? ""}
          onChange={(e) => onChange({ label: e.target.value || undefined })}
          style={{ width: "100%", padding: 4, marginTop: 2 }}
        />
      </label>
      <label style={{ display: "flex", gap: 6, alignItems: "center", marginTop: 4 }}>
        <input
          type="checkbox"
          checked={field.required}
          onChange={(e) => onChange({ required: e.target.checked })}
        />
        Required
      </label>
      <div style={{ marginTop: 6, color: "#57606a" }}>
        Page {field.pageIndex + 1} \u2013{" "}
        {Math.round(field.width)}\u00d7{Math.round(field.height)} pt at (
        {Math.round(field.x)}, {Math.round(field.y)})
      </div>
      <button
        type="button"
        onClick={onDelete}
        style={{
          marginTop: 8,
          padding: "4px 8px",
          background: "#cf222e",
          color: "#fff",
          border: "none",
          borderRadius: 3,
          cursor: "pointer",
        }}
      >
        Delete field
      </button>
    </div>
  );
}

function genId(): string {
  const bytes = new Uint8Array(16);
  crypto.getRandomValues(bytes);
  return Array.from(bytes).map((b) => b.toString(16).padStart(2, "0")).join("");
}
