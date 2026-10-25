import type { ComponentProps } from 'react';
import { Link as RouterLink } from 'react-router';
import { Link } from '@alfalab/core-components/link';

export function AppLink(props: ComponentProps<typeof RouterLink>) {
  return (
    <Link Component={RouterLink} view="default" underline={false} {...props}>
      {props.children}
    </Link>
  );
}
