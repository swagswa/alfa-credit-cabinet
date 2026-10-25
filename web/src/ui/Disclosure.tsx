import { useId, useState, type ReactNode } from 'react';
import { Collapse } from '@alfalab/core-components/collapse';
import { Button } from '@alfalab/core-components/button';
import { ChevronDownSIcon } from '@alfalab/icons-glyph/ChevronDownSIcon';

export function Disclosure({
  label,
  children,
  className = '',
  id,
}: {
  label: ReactNode;
  children: ReactNode;
  className?: string;
  id?: string;
}) {
  const [expanded, setExpanded] = useState(false);
  const contentId = useId();
  return (
    <div id={id} className={`kit-disclosure ${className}`}>
      <Button
        view="text"
        size={40}
        className="kit-disclosure-trigger"
        aria-expanded={expanded}
        aria-controls={contentId}
        onClick={() => setExpanded((value) => !value)}
        rightAddons={<ChevronDownSIcon aria-hidden="true" />}
      >
        {label}
      </Button>
      <Collapse expanded={expanded} className="kit-disclosure-collapse">
        <div id={contentId} inert={!expanded} aria-hidden={!expanded}>
          {children}
        </div>
      </Collapse>
    </div>
  );
}
