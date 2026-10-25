import { ArrowLeftSIcon } from '@alfalab/icons-glyph/ArrowLeftSIcon';
import { HouseHomeMIcon } from '@alfalab/icons-glyph/HouseHomeMIcon';
import { DocumentLinesLineSIcon } from '@alfalab/icons-glyph/DocumentLinesLineSIcon';
import { PlusSIcon } from '@alfalab/icons-glyph/PlusSIcon';
import { ArrowsCwCircleSIcon } from '@alfalab/icons-glyph/ArrowsCwCircleSIcon';
import { CheckmarkSIcon } from '@alfalab/icons-glyph/CheckmarkSIcon';
import { ArrowRightSIcon } from '@alfalab/icons-glyph/ArrowRightSIcon';
import { UserRoundedSIcon } from '@alfalab/icons-glyph/UserRoundedSIcon';
import { LogOutMIcon } from '@alfalab/icons-glyph/LogOutMIcon';
import { Status } from '@alfalab/core-components/status';
import { AppLink as Link } from './AppLink';
import { Typography } from '@alfalab/core-components/typography';
import type { ReactNode } from 'react';

import { labels } from '../api/presentation';
import type { components } from '../api/schema.gen';
import { motion, useMotionDisabled } from '../motion';
import { springs } from '../motion/tokens';

const icons = {
  back: ArrowLeftSIcon,
  overview: HouseHomeMIcon,
  applications: DocumentLinesLineSIcon,
  new: PlusSIcon,
  refresh: ArrowsCwCircleSIcon,
  check: CheckmarkSIcon,
  next: ArrowRightSIcon,
  user: UserRoundedSIcon,
  profile: UserRoundedSIcon,
  logout: LogOutMIcon,
};
export function Icon({ name }: { name: keyof typeof icons }) {
  const Glyph = icons[name];
  return <Glyph className="workspace-icon" width={20} height={20} aria-hidden="true" focusable="false" />;
}
export function PageHeading({
  title,
  description,
  back,
  children,
}: {
  title: ReactNode;
  description?: ReactNode;
  back?: string;
  children?: ReactNode;
}) {
  const reduced = useMotionDisabled();
  return (
    <div className="workspace-heading application-toolbar">
      <div className="workspace-heading-title">
        {back && (
          <Link className="heading-back" to={back} aria-label="К списку заявок">
            <motion.span
              initial={reduced ? false : { opacity: 0 }}
              animate={{ opacity: 1 }}
              transition={{ duration: reduced ? 0 : 0.16 }}
            >
              <Icon name="back" />
            </motion.span>
          </Link>
        )}
        <motion.div
          initial={false}
          layout={reduced ? false : 'position'}
          layoutId={!reduced && typeof title === 'string' ? `page-title:${title}` : undefined}
          transition={reduced ? { duration: 0 } : springs.layout}
        >
          <Typography.Title tag="h1" view="medium" font="system">
            {title}
          </Typography.Title>
        </motion.div>
      </div>
      <div className="workspace-heading-actions">{children}</div>
      {description && (
        <Typography.Text tag="p" view="primary-small">
          {description}
        </Typography.Text>
      )}
    </div>
  );
}
export function SectionHeading({ title, children }: { title: ReactNode; children?: ReactNode }) {
  return (
    <div className="dashboard-section-heading">
      <Typography.Title tag="h2" view="small" font="system" defaultMargins>
        {title}
      </Typography.Title>
      {children}
    </div>
  );
}
export function StatusBadge({ status }: { status: components['schemas']['ApplicationStatus'] }) {
  const color = ['approved', 'issued', 'closed'].includes(status)
    ? 'green'
    : status === 'rejected'
      ? 'red'
      : ['need_info', 'signing', 'accepted', 'expired'].includes(status)
        ? 'orange'
        : status === 'draft' || status === 'cancelled_by_client'
          ? 'grey'
          : 'blue';
  return (
    <Status
      className={`application-status status-${status}`}
      view="muted-alt"
      color={color}
      shape="rounded"
      size={24}
      uppercase={false}
    >
      {labels[status]}
    </Status>
  );
}
export function FieldPair({ children }: { children: ReactNode }) {
  return <div className="field-pair">{children}</div>;
}
