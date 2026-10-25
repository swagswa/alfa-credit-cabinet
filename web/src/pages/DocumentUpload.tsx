import { useState, type FormEvent } from 'react';
import { Attach } from '@alfalab/core-components/attach';
import { Typography } from '@alfalab/core-components/typography';
import { Button } from '../motion';
import { kinds } from '../api/presentation';
import type { components } from '../api/schema.gen';

type Kind = components['schemas']['DocumentKind'];
export function DocumentUpload({
  kind,
  busy,
  uploaded,
  onUpload,
}: {
  kind: Kind;
  busy: boolean;
  uploaded: boolean;
  onUpload: (file: File, kind: Kind) => Promise<boolean>;
}) {
  const [file, setFile] = useState<File | null>(null);
  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (file && (await onUpload(file, kind))) setFile(null);
  }
  return (
    <form className="document-kind-upload" onSubmit={submit}>
      <Attach
        aria-label={`Файл: ${kinds[kind]}`}
        buttonContent={uploaded ? 'Выбрать другой файл' : 'Выбрать файл'}
        accept="application/pdf,image/png,image/jpeg"
        value={file ? [file] : null}
        onChange={(_, payload) => setFile(payload.files[0] ?? null)}
        onClear={() => setFile(null)}
        disabled={busy}
      />
      <Typography.Text tag="p" view="secondary-small" color="secondary">
        PDF, PNG или JPEG, до 10 МБ и 5 страниц.
      </Typography.Text>
      <Button type="submit" disabled={busy || !file}>
        Загрузить: {kinds[kind]}
      </Button>
    </form>
  );
}
