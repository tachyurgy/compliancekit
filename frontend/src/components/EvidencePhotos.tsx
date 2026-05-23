import { useMemo, useRef, useState } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { Camera, ImagePlus, Pencil, Trash2, X } from 'lucide-react';

import Button from '@/components/common/Button';
import { toast } from '@/components/common/Toast';
import {
  evidencePhotosApi,
  type EvidencePhoto,
  type EvidencePhotoSubjectKind,
} from '@/api/evidencePhotos';

// REQ-038. Drop in next to anything you want to attach photos to:
//   <EvidencePhotos subjectKind="drill" subjectId={drill.id} />
//
// The component renders a 4-column thumbnail grid + an "Add a photo"
// affordance. Each thumbnail is hoverable for delete + caption-edit.
// Empty state is a calm one-liner — no scary copy.

interface Props {
  subjectKind: EvidencePhotoSubjectKind;
  subjectId: string;
  /** Show a tighter row (no card padding) — used inside table cells. */
  compact?: boolean;
  /** Heading text. Defaults to "Photos". */
  title?: string;
}

export default function EvidencePhotos({
  subjectKind,
  subjectId,
  compact,
  title = 'Photos',
}: Props): JSX.Element {
  const qc = useQueryClient();
  const fileInput = useRef<HTMLInputElement>(null);
  const [editingId, setEditingId] = useState<string | null>(null);
  const [draftCaption, setDraftCaption] = useState('');

  const queryKey = useMemo(
    () => ['evidence-photos', subjectKind, subjectId],
    [subjectKind, subjectId],
  );

  const { data: photos = [], isLoading } = useQuery({
    queryKey,
    queryFn: () => evidencePhotosApi.list(subjectKind, subjectId),
    enabled: Boolean(subjectId),
  });

  const upload = useMutation({
    mutationFn: (files: File[]) =>
      evidencePhotosApi.upload(subjectKind, subjectId, files),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey });
      toast.success('Photo added');
    },
    onError: (err) => {
      toast.error(err instanceof Error ? err.message : 'Could not upload photo');
    },
  });

  const update = useMutation({
    mutationFn: ({ id, caption }: { id: string; caption: string }) =>
      evidencePhotosApi.update(id, caption),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey });
      toast.success('Caption saved');
      setEditingId(null);
    },
    onError: (err) => {
      toast.error(err instanceof Error ? err.message : 'Could not save caption');
    },
  });

  const remove = useMutation({
    mutationFn: (id: string) => evidencePhotosApi.remove(id),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey });
      toast.success('Photo removed');
    },
    onError: (err) => {
      toast.error(err instanceof Error ? err.message : 'Could not remove photo');
    },
  });

  function onPickFiles(ev: React.ChangeEvent<HTMLInputElement>) {
    const files = Array.from(ev.target.files ?? []);
    if (files.length === 0) return;
    upload.mutate(files);
    // Reset input so re-uploading the same filename works.
    ev.target.value = '';
  }

  return (
    <div className={compact ? '' : 'mt-3'}>
      <div className="flex items-center justify-between gap-2 mb-2">
        <div className="flex items-center gap-2 text-sm font-medium text-ink-800">
          <Camera className="h-4 w-4 text-ink-500" />
          {title}
          {photos.length > 0 && (
            <span className="text-xs text-ink-500">({photos.length})</span>
          )}
        </div>
        <Button
          size="sm"
          variant="secondary"
          leftIcon={<ImagePlus className="h-4 w-4" />}
          loading={upload.isPending}
          onClick={() => fileInput.current?.click()}
        >
          Add a photo
        </Button>
        <input
          ref={fileInput}
          type="file"
          accept="image/jpeg,image/png,image/webp,image/heic,image/gif"
          multiple
          hidden
          onChange={onPickFiles}
        />
      </div>

      {isLoading ? (
        <div className="grid grid-cols-2 sm:grid-cols-3 md:grid-cols-4 gap-2">
          {Array.from({ length: 3 }).map((_, i) => (
            <div key={i} className="skeleton aspect-square w-full rounded-lg" />
          ))}
        </div>
      ) : photos.length === 0 ? (
        <p className="text-xs text-ink-500 italic">
          No photos yet. Snap a quick shot — the signed muster sheet, the
          posted notice, anything that helps an inspector see it's done.
        </p>
      ) : (
        <ul className="grid grid-cols-2 sm:grid-cols-3 md:grid-cols-4 gap-2">
          {photos.map((p) => (
            <EvidenceThumb
              key={p.id}
              photo={p}
              isEditing={editingId === p.id}
              draftCaption={editingId === p.id ? draftCaption : p.caption}
              onStartEdit={() => {
                setEditingId(p.id);
                setDraftCaption(p.caption);
              }}
              onChangeCaption={(v) => setDraftCaption(v)}
              onSaveCaption={() =>
                update.mutate({ id: p.id, caption: draftCaption })
              }
              onCancelEdit={() => setEditingId(null)}
              onDelete={() => {
                if (confirm('Remove this photo?')) remove.mutate(p.id);
              }}
              busy={remove.isPending || update.isPending}
            />
          ))}
        </ul>
      )}
    </div>
  );
}

function EvidenceThumb({
  photo,
  isEditing,
  draftCaption,
  onStartEdit,
  onChangeCaption,
  onSaveCaption,
  onCancelEdit,
  onDelete,
  busy,
}: {
  photo: EvidencePhoto;
  isEditing: boolean;
  draftCaption: string;
  onStartEdit: () => void;
  onChangeCaption: (v: string) => void;
  onSaveCaption: () => void;
  onCancelEdit: () => void;
  onDelete: () => void;
  busy: boolean;
}): JSX.Element {
  return (
    <li className="group relative overflow-hidden rounded-lg border border-line-soft bg-white">
      <div className="aspect-square w-full bg-surface-sunken">
        {photo.download_url ? (
          <img
            src={photo.download_url}
            alt={photo.caption || 'Evidence photo'}
            className="h-full w-full object-cover"
            loading="lazy"
          />
        ) : (
          <div className="grid h-full w-full place-items-center text-ink-400">
            <Camera className="h-6 w-6" />
          </div>
        )}
      </div>

      {/* Caption + actions */}
      <div className="p-2">
        {isEditing ? (
          <div className="space-y-2">
            <input
              type="text"
              value={draftCaption}
              onChange={(e) => onChangeCaption(e.target.value)}
              maxLength={200}
              placeholder="Caption (optional)"
              className="block w-full rounded border border-line text-xs px-2 py-1"
              autoFocus
            />
            <div className="flex justify-end gap-1">
              <button
                type="button"
                onClick={onCancelEdit}
                disabled={busy}
                className="text-xs text-ink-500 hover:text-ink-700 px-2 py-0.5"
              >
                Cancel
              </button>
              <button
                type="button"
                onClick={onSaveCaption}
                disabled={busy}
                className="text-xs text-brand-700 hover:text-brand-800 font-medium px-2 py-0.5"
              >
                Save
              </button>
            </div>
          </div>
        ) : (
          <p className="text-xs text-ink-700 truncate" title={photo.caption}>
            {photo.caption || (
              <span className="italic text-ink-400">No caption</span>
            )}
          </p>
        )}
      </div>

      {/* Hover affordances */}
      {!isEditing && (
        <div className="pointer-events-none absolute right-1 top-1 flex gap-1 opacity-0 transition-opacity group-hover:opacity-100">
          <button
            type="button"
            onClick={onStartEdit}
            aria-label="Edit caption"
            className="pointer-events-auto rounded-full bg-white/95 p-1.5 shadow text-ink-700 hover:text-brand-700"
          >
            <Pencil className="h-3.5 w-3.5" />
          </button>
          <button
            type="button"
            onClick={onDelete}
            aria-label="Remove photo"
            disabled={busy}
            className="pointer-events-auto rounded-full bg-white/95 p-1.5 shadow text-ink-700 hover:text-critical-600"
          >
            {busy ? <X className="h-3.5 w-3.5" /> : <Trash2 className="h-3.5 w-3.5" />}
          </button>
        </div>
      )}
    </li>
  );
}
