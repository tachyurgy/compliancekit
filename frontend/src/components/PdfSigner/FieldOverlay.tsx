import type { CSSProperties, ReactNode } from "react";
import type { Field } from "./types";

export interface FieldOverlayProps {
  /** Fields filtered to only those on this page. */
  fields: Field[];
  /** Actual rendered width in CSS pixels. */
  renderedWidth: number;
  /** Actual rendered height in CSS pixels. */
  renderedHeight: number;
  /** Native PDF page width in PDF points. */
  pageWidth: number;
  /** Native PDF page height in PDF points. */
  pageHeight: number;
  /** Map of field id -> true if already filled. */
  filledFieldIds: Set<string>;
  onFieldClick?: (field: Field) => void;
  /** Optional: authoring mode — fields become draggable. */
  editable?: boolean;
  onFieldMove?: (id: string, next: { x: number; y: number }) => void;
  onFieldResize?: (id: string, next: { width: number; height: number }) => void;
  onFieldDelete?: (id: string) => void;
  selectedFieldId?: string;
  onSelectField?: (id: string | null) => void;
}

/**
 * Renders absolute-positioned boxes over a PDF page, translating from
 * PDF-point (bottom-left origin) to screen (top-left origin) coordinates.
 */
export function FieldOverlay({
  fields,
  renderedWidth,
  renderedHeight,
  pageWidth,
  pageHeight,
  filledFieldIds,
  onFieldClick,
  editable,
  onFieldMove,
  onFieldResize,
  onFieldDelete,
  selectedFieldId,
  onSelectField,
}: FieldOverlayProps): ReactNode {
  const scaleX = renderedWidth / pageWidth;
  const scaleY = renderedHeight / pageHeight;

  return (
    <div
      className="pdfsign-overlay"
      style={{
        position: "absolute",
        inset: 0,
        width: renderedWidth,
        height: renderedHeight,
        pointerEvents: editable ? "auto" : undefined,
      }}
      onMouseDown={(e) => {
        if (editable && e.target === e.currentTarget) {
          onSelectField?.(null);
        }
      }}
    >
      {fields.map((f) => {
        const filled = filledFieldIds.has(f.id);
        const left = f.x * scaleX;
        const top = (pageHeight - f.y - f.height) * scaleY;
        const width = f.width * scaleX;
        const height = f.height * scaleY;
        const selected = selectedFieldId === f.id;
        const style: CSSProperties = {
          position: "absolute",
          left,
          top,
          width,
          height,
          border: filled
            ? "1px dashed #1f883d"
            : f.required
              ? "2px solid #0969da"
              : "2px dashed #8250df",
          background: filled
            ? "rgba(31,136,61,0.06)"
            : "rgba(9,105,218,0.08)",
          boxShadow: selected ? "0 0 0 2px #0969da" : undefined,
          fontSize: 11,
          color: "#24292f",
          display: "flex",
          alignItems: "center",
          justifyContent: "center",
          cursor: editable ? "move" : "pointer",
          userSelect: "none",
          borderRadius: 3,
        };
        return (
          <div
            key={f.id}
            role="button"
            tabIndex={0}
            aria-label={`${f.type} field${f.label ? ` ${f.label}` : ""}`}
            style={style}
            onClick={(e) => {
              e.stopPropagation();
              if (editable) {
                onSelectField?.(f.id);
              } else {
                onFieldClick?.(f);
              }
            }}
            onMouseDown={(e) => {
              if (!editable || !onFieldMove) return;
              e.stopPropagation();
              onSelectField?.(f.id);
              const startX = e.clientX;
              const startY = e.clientY;
              const origX = f.x;
              const origY = f.y;
              const onMove = (ev: MouseEvent) => {
                const dx = (ev.clientX - startX) / scaleX;
                const dy = (ev.clientY - startY) / scaleY;
                // screen-down is pdf-up flipped; so subtract dy
                onFieldMove(f.id, {
                  x: Math.max(0, origX + dx),
                  y: Math.max(0, origY - dy),
                });
              };
              const onUp = () => {
                window.removeEventListener("mousemove", onMove);
                window.removeEventListener("mouseup", onUp);
              };
              window.addEventListener("mousemove", onMove);
              window.addEventListener("mouseup", onUp);
            }}
          >
            <span style={{ pointerEvents: "none" }}>
              {filled ? "\u2713 " : ""}
              {f.label ?? labelFor(f.type)}
            </span>
            {editable && selected && (
              <>
                <button
                  type="button"
                  onClick={(e) => {
                    e.stopPropagation();
                    onFieldDelete?.(f.id);
                  }}
                  style={{
                    position: "absolute",
                    top: -22,
                    right: -2,
                    fontSize: 11,
                    padding: "1px 6px",
                    background: "#cf222e",
                    color: "#fff",
                    border: "none",
                    borderRadius: 3,
                    cursor: "pointer",
                  }}
                >
                  delete
                </button>
                <ResizeHandle
                  onDrag={(dx, dy) => {
                    onFieldResize?.(f.id, {
                      width: Math.max(20, f.width + dx / scaleX),
                      height: Math.max(14, f.height + dy / scaleY),
                    });
                  }}
                />
              </>
            )}
          </div>
        );
      })}
    </div>
  );
}

function ResizeHandle({ onDrag }: { onDrag: (dx: number, dy: number) => void }) {
  return (
    <div
      onMouseDown={(e) => {
        e.stopPropagation();
        const startX = e.clientX;
        const startY = e.clientY;
        const onMove = (ev: MouseEvent) => {
          onDrag(ev.clientX - startX, ev.clientY - startY);
        };
        const onUp = () => {
          window.removeEventListener("mousemove", onMove);
          window.removeEventListener("mouseup", onUp);
        };
        window.addEventListener("mousemove", onMove);
        window.addEventListener("mouseup", onUp);
      }}
      style={{
        position: "absolute",
        right: -4,
        bottom: -4,
        width: 10,
        height: 10,
        background: "#0969da",
        cursor: "nwse-resize",
        borderRadius: 2,
      }}
    />
  );
}

function labelFor(t: Field["type"]): string {
  switch (t) {
    case "signature":
      return "Sign here";
    case "initial":
      return "Initial";
    case "date":
      return "Date";
    case "text":
      return "Text";
    case "checkbox":
      return "\u2610";
  }
}
