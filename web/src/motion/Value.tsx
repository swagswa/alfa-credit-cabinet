import { motion, AnimatePresence, useMotionDisabled } from './index';
import { springs } from './tokens';

// Render the actual server value, never intermediate financial amounts.
export function Value({ text }: { text: string }) {
  const reduced = useMotionDisabled();
  return <span className="motion-value" aria-label={text}><span aria-hidden="true">{Array.from(text).map((character, index) => <span className="value-cell" key={index}><AnimatePresence initial={false} mode="popLayout"><motion.span key={character} initial={reduced ? false : { y: '35%', opacity: 0 }} animate={{ y: 0, opacity: 1 }} exit={{ y: '-35%', opacity: 0 }} transition={reduced ? { duration: 0 } : springs.panel}>{character}</motion.span></AnimatePresence></span>)}</span></span>;
}
