import type { ReactNode } from 'react';
import { Link } from 'react-router';
import { labels } from '../api/presentation';
import type { components } from '../api/schema.gen';
import { motion, useMotionDisabled } from '../motion';
import { springs } from '../motion/tokens';

export function Icon({ name }: { name: 'back' | 'overview' | 'applications' | 'new' | 'refresh' | 'check' }) {
  const paths = { back: 'm14 6-6 6 6 6M8 12h12', overview: 'm3.5 10 7-6a2.3 2.3 0 0 1 3 0l7 6M5.5 9v9a2 2 0 0 0 2 2h9a2 2 0 0 0 2-2V9M9.5 20v-5a2.5 2.5 0 0 1 5 0v5', applications: 'M8 4h9a3 3 0 0 1 3 3v9M7 8h7a3 3 0 0 1 3 3v7a3 3 0 0 1-3 3H7a3 3 0 0 1-3-3v-7a3 3 0 0 1 3-3Zm1 5h5m-5 4h3', new: 'M12 5v14M5 12h14', refresh: 'M21 12a9 9 0 1 1-2.64-6.36L21 8M21 3v5h-5', check: 'm5 12.5 4.5 4.5L19 7' };
  return <svg className="workspace-icon" width="20" height="20" viewBox="0 0 24 24" fill="none" aria-hidden="true"><path d={paths[name]} stroke="currentColor" strokeWidth="1.6" strokeLinecap="round" strokeLinejoin="round" /></svg>;
}
export function PageHeading({ title, description, back, children }: { title: ReactNode; description?: ReactNode; back?: string; children?: ReactNode }) {
  const reduced = useMotionDisabled();
  return <div className="workspace-heading application-toolbar"><div className="workspace-heading-title">{back && <Link className="heading-back" to={back} aria-label="К списку заявок"><motion.span initial={reduced ? false : { opacity: 0 }} animate={{ opacity: 1 }} transition={{ duration: reduced ? 0 : 0.16 }}><Icon name="back" /></motion.span></Link>}<motion.h1 initial={false} layout={reduced ? false : 'position'} layoutId={!reduced && typeof title === 'string' ? `page-title:${title}` : undefined} transition={reduced ? {duration:0} : springs.layout}>{title}</motion.h1></div><div className="workspace-heading-actions">{children}</div>{description && <p>{description}</p>}</div>;
}
export function SectionHeading({ title, children }: { title: ReactNode; children?: ReactNode }) {
  return <div className="dashboard-section-heading"><h2>{title}</h2>{children}</div>;
}
export function StatusBadge({ status }: { status: components['schemas']['ApplicationStatus'] }) {
  return <span className={`status-badge status-${status}`}>{labels[status]}</span>;
}
export function FieldPair({ children }: { children: ReactNode }) {
  return <div className="field-pair">{children}</div>;
}
