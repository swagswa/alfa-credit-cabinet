import type { HTMLAttributes } from 'react';
import { Plate } from '@alfalab/core-components/plate';

export function InlineNotice({ children, ...props }: HTMLAttributes<HTMLDivElement>) {
  return (
    <div {...props} role="alert">
      <Plate view="negative" rounded limitContentWidth={false}>
        {children}
      </Plate>
    </div>
  );
}
