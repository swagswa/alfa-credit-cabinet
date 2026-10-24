import {
  Children,
  useEffect,
  useRef,
  useState,
  useSyncExternalStore,
  type ComponentProps,
  type ReactNode,
} from 'react';
import {
  motion,
  MotionConfig,
  AnimatePresence,
  LayoutGroup,
  useAnimate,
  useIsPresent,
  type HTMLMotionProps,
} from 'motion/react';
import { Button as KitButton } from '@alfalab/core-components/button';
import { Checkbox as KitCheckbox } from '@alfalab/core-components/checkbox';
import { Select as KitSelect } from '@alfalab/core-components/select';
import { NavLink, useLocation } from 'react-router';
import { springs, timing } from './tokens';

const listeners = new Set<() => void>();
let preference: MediaQueryList | undefined;
const getPreference = () => (preference ??= matchMedia('(prefers-reduced-motion: reduce)'));
const changed = () => listeners.forEach((notify) => notify());
const subscribe = (notify: () => void) => {
  const query = getPreference();
  if (!listeners.size) query.addEventListener('change', changed);
  listeners.add(notify);
  return () => {
    listeners.delete(notify);
    if (!listeners.size) query.removeEventListener('change', changed);
  };
};
export function useMotionDisabled() {
  return useSyncExternalStore(
    subscribe,
    () => getPreference().matches,
    () => true,
  );
}
export function MotionProvider({ children }: { children: ReactNode }) {
  const reduced = useMotionDisabled();
  return (
    <MotionConfig reducedMotion={reduced ? 'always' : 'never'} transition={reduced ? { duration: 0 } : springs.layout}>
      {children}
    </MotionConfig>
  );
}
const PhysicalButton = motion.create(KitButton);
export function Button(props: ComponentProps<typeof PhysicalButton> & { disabled?: boolean }) {
  const reduced = useMotionDisabled();
  return (
    <PhysicalButton
      {...props}
      whileTap={reduced || ('disabled' in props && props.disabled) || props.loading ? undefined : { scale: 0.975 }}
      transition={reduced ? { duration: 0 } : springs.press}
    />
  );
}
export function Checkbox(props: ComponentProps<typeof KitCheckbox>) {
  const reduced = useMotionDisabled();
  const [scope, animate] = useAnimate<HTMLLabelElement>();
  const previous = useRef(props.checked);
  const pointer = useRef(false);
  useEffect(() => {
    if (previous.current !== props.checked && !reduced && pointer.current) {
      const control = scope.current?.querySelector('.physical-check-box');
      if (control) void animate(control, { scale: [0.9, 1] }, springs.press);
    }
    previous.current = props.checked;
    pointer.current = false;
  }, [props.checked, reduced, animate, scope]);
  return (
    <KitCheckbox
      {...props}
      ref={scope}
      boxClassName={`physical-check-box ${props.boxClassName ?? ''}`}
      labelProps={{
        ...props.labelProps,
        onPointerDown: (event) => {
          pointer.current = true;
          props.labelProps?.onPointerDown?.(event);
        },
        onKeyDown: (event) => {
          pointer.current = false;
          props.labelProps?.onKeyDown?.(event);
        },
      }}
    />
  );
}
export function Select(props: ComponentProps<typeof KitSelect>) {
  const reduced = useMotionDisabled();
  return (
    <KitSelect
      {...props}
      popperClassName={`motion-popover ${props.popperClassName ?? ''}`}
      popoverProps={{ ...props.popoverProps, transitionDuration: reduced ? '0ms' : '150ms' }}
    />
  );
}
export function MotionNavLink({ children, ...props }: ComponentProps<typeof NavLink>) {
  const reduced = useMotionDisabled();
  return (
    <NavLink {...props}>
      {(state) => (
        <>
          {state.isActive && (
            <motion.span
              className="nav-selection"
              layoutId="cabinet-selection"
              transition={reduced ? { duration: 0 } : springs.layout}
              aria-hidden="true"
            />
          )}
          <span className="nav-caption">{typeof children === 'function' ? children(state) : children}</span>
        </>
      )}
    </NavLink>
  );
}
export function RouteSurface({ children }: { children: ReactNode }) {
  const { pathname } = useLocation();
  return (
    <LayoutGroup>
      <div key={pathname} className="route-surface">
        {children}
      </div>
    </LayoutGroup>
  );
}
export function MotionDetails({ children, ...props }: ComponentProps<'details'>) {
  const reduced = useMotionDisabled();
  const [expanded, setExpanded] = useState(!!props.open);
  const [visible, setVisible] = useState(!!props.open);
  const [summary, ...content] = Children.toArray(children);
  return (
    <details
      {...props}
      open={visible}
      onClick={(event) => {
        const target = event.target as Element;
        if (target.closest('summary')?.parentElement !== event.currentTarget) return;
        event.preventDefault();
        if (!expanded) setVisible(true);
        setExpanded(!expanded);
      }}
    >
      {summary}
      <motion.div
        className="disclosure-content"
        inert={!expanded}
        aria-hidden={!expanded}
        initial={false}
        animate={{ height: expanded ? 'auto' : 0, opacity: expanded ? 1 : 0 }}
        transition={reduced ? { duration: 0 } : { height: springs.panel, opacity: { duration: timing.response } }}
        onAnimationComplete={() => {
          if (!expanded) setVisible(false);
        }}
      >
        {content}
      </motion.div>
    </details>
  );
}
export function StatusText({ value, children }: { value: string; children: ReactNode }) {
  const reduced = useMotionDisabled();
  return (
    <motion.span
      className="status-transition"
      layout="position"
      transition={reduced ? { duration: 0 } : springs.layout}
    >
      <AnimatePresence initial={false} mode="popLayout">
        <motion.span
          key={value}
          initial={reduced ? false : { opacity: 0 }}
          animate={{ opacity: 1 }}
          exit={{ opacity: 0 }}
          transition={{ duration: reduced ? 0 : timing.response }}
        >
          {children}
        </motion.span>
      </AnimatePresence>
    </motion.span>
  );
}
export function ListItem(props: HTMLMotionProps<'li'>) {
  const present = useIsPresent();
  const reduced = useMotionDisabled();
  return (
    <motion.li
      {...props}
      aria-hidden={!present || undefined}
      inert={!present}
      layout={reduced ? false : (props.layout ?? 'position')}
      initial={reduced ? false : { opacity: 0 }}
      animate={{ opacity: 1 }}
      exit={{ opacity: 0 }}
      transition={reduced ? { duration: 0 } : { ...springs.layout, opacity: { duration: timing.exit } }}
    />
  );
}
export { motion, AnimatePresence, LayoutGroup };
