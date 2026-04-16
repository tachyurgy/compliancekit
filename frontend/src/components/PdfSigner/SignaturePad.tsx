import SignaturePadLib from "signature_pad";
import {
  forwardRef,
  useEffect,
  useImperativeHandle,
  useRef,
  useState,
} from "react";

export interface SignaturePadHandle {
  clear(): void;
  undo(): void;
  isEmpty(): boolean;
  /** Returns a trimmed PNG blob at device pixel ratio. */
  getPng(): Promise<Blob>;
  /** Returns the raw data URL (used for pdf-lib embed). */
  getDataUrl(): string | null;
}

export interface SignaturePadProps {
  width?: number;
  height?: number;
  penColor?: string;
  onChange?: (empty: boolean) => void;
  className?: string;
}

export const SignaturePad = forwardRef<SignaturePadHandle, SignaturePadProps>(
  function SignaturePad(
    { width = 560, height = 180, penColor = "#111", onChange, className },
    ref,
  ) {
    const canvasRef = useRef<HTMLCanvasElement | null>(null);
    const padRef = useRef<SignaturePadLib | null>(null);
    const [empty, setEmpty] = useState(true);

    useEffect(() => {
      const canvas = canvasRef.current;
      if (!canvas) return;
      const ratio = Math.max(window.devicePixelRatio || 1, 1);
      canvas.width = width * ratio;
      canvas.height = height * ratio;
      canvas.style.width = `${width}px`;
      canvas.style.height = `${height}px`;
      const ctx = canvas.getContext("2d");
      if (ctx) ctx.scale(ratio, ratio);

      const pad = new SignaturePadLib(canvas, {
        penColor,
        backgroundColor: "rgba(0,0,0,0)",
        minWidth: 0.6,
        maxWidth: 2.2,
        throttle: 16,
      });
      pad.addEventListener("endStroke", () => {
        const isEmpty = pad.isEmpty();
        setEmpty(isEmpty);
        onChange?.(isEmpty);
      });
      padRef.current = pad;
      return () => {
        pad.off();
        padRef.current = null;
      };
    }, [width, height, penColor, onChange]);

    useImperativeHandle(ref, () => ({
      clear() {
        padRef.current?.clear();
        setEmpty(true);
        onChange?.(true);
      },
      undo() {
        const pad = padRef.current;
        if (!pad) return;
        const data = pad.toData();
        if (data.length > 0) {
          data.pop();
          pad.fromData(data);
          const isEmpty = pad.isEmpty();
          setEmpty(isEmpty);
          onChange?.(isEmpty);
        }
      },
      isEmpty() {
        return padRef.current?.isEmpty() ?? true;
      },
      async getPng() {
        const pad = padRef.current;
        if (!pad || pad.isEmpty()) {
          throw new Error("signature pad is empty");
        }
        const dataUrl = pad.toDataURL("image/png");
        const resp = await fetch(dataUrl);
        return await resp.blob();
      },
      getDataUrl() {
        const pad = padRef.current;
        if (!pad || pad.isEmpty()) return null;
        return pad.toDataURL("image/png");
      },
    }));

    return (
      <div className={className ?? "pdfsign-pad"}>
        <canvas
          ref={canvasRef}
          style={{
            border: "1px solid #d0d7de",
            borderRadius: 6,
            touchAction: "none",
            background: "#fff",
            display: "block",
          }}
        />
        <div style={{ marginTop: 6, fontSize: 12, color: "#6a737d" }}>
          {empty ? "Sign above using mouse, finger, or stylus." : "Captured."}
        </div>
      </div>
    );
  },
);
