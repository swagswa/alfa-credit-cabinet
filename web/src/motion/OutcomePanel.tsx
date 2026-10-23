import type { ReactNode } from 'react';
import { motion, useMotionDisabled } from './index';
import { springs, timing, easeOut } from './tokens';

export function OutcomePanel({ rejected = false, children }: { rejected?: boolean; children: ReactNode }) {
  const reduced = useMotionDisabled();
  const label = rejected ? 'rejection-title' : 'success-title';
  return <motion.section className={`result-panel ${rejected ? 'rejection-result' : 'success-result'}`} aria-labelledby={label} initial={reduced ? false : 'waiting'} whileInView="ready" viewport={{ once: true, amount: 0.15 }}>
    <motion.span className={rejected ? 'result-wave' : 'success-wave'} aria-hidden="true" variants={{ waiting: { transform: 'scale(0.025)', opacity: 0 }, ready: { transform: 'scale(1)', opacity: 1 } }} transition={{ duration: reduced ? 0 : 0.8, ease: easeOut }} />
    <motion.div className="result-icon" aria-hidden="true" variants={{ waiting: { scale: 0.82, opacity: 0 }, ready: { scale: 1, opacity: 1 } }} transition={reduced ? { duration: 0 } : rejected ? springs.panel : springs.result}><svg viewBox="0 0 32 32" fill="none"><motion.path d={rejected ? 'M10 10l12 12M22 10L10 22' : 'm8 16 5.5 5.5L24 10'} stroke="currentColor" strokeWidth="2.5" strokeLinecap="round" strokeLinejoin="round" variants={{ waiting: { pathLength: 0 }, ready: { pathLength: 1 } }} transition={{ duration: reduced ? 0 : 0.32, delay: reduced ? 0 : 0.12, ease: easeOut }}/></svg></motion.div>
    <motion.div className="outcome-content" variants={{ waiting: { opacity: 0 }, ready: { opacity: 1 } }} transition={{ duration: reduced ? 0 : timing.transition, delay: reduced ? 0 : 0.18 }}>{children}</motion.div>
  </motion.section>;
}
