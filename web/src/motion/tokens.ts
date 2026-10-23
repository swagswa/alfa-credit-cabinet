// Seconds; short responses never wait for a decorative sequence.
export const springs = {
  press: { type: 'spring', stiffness: 650, damping: 34, mass: 0.65 },
  layout: { type: 'spring', stiffness: 440, damping: 38, mass: 0.85 },
  panel: { type: 'spring', stiffness: 380, damping: 36, mass: 0.9 },
  result: { type: 'spring', stiffness: 300, damping: 23, mass: 0.8 },
} as const;
export const timing = { response: 0.12, transition: 0.2, exit: 0.16 } as const;
export const easeOut = [0.22, 1, 0.36, 1] as const;
