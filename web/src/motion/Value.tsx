import { motion, useMotionDisabled } from './index';

// Сумма остаётся одной строкой и читается целиком, а не по символам.
export function Value({ text }: { text: string }) {
  const reduced = useMotionDisabled();
  return (
    <motion.span
      key={text}
      className="motion-value"
      initial={reduced ? false : { opacity: 0.7 }}
      animate={{ opacity: 1 }}
      transition={{ duration: reduced ? 0 : 0.14 }}
    >
      {text}
    </motion.span>
  );
}
