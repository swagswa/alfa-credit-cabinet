import { useEffect, useRef, useState } from 'react';
import { createPortal } from 'react-dom';
import { motion, AnimatePresence, useMotionDisabled } from './motion';
import { springs, timing } from './motion/tokens';

type Notice = { id: string; message: string; tone: 'error' | 'success'; close?: () => void };
export function notify(message: string, tone: Notice['tone'] = 'success', close?: () => void) {
  const id = crypto.randomUUID();
  window.dispatchEvent(new CustomEvent<Notice>('cabinet-notice', { detail: { id, message, tone, close } }));
  return id;
}
export function NoticeHost() {
  const [notices, setNotices] = useState<Notice[]>([]);
  const reduced = useMotionDisabled();
  useEffect(() => {
    const timers = new Map<string, ReturnType<typeof setTimeout>>();
    const receive = (event: Event) => {
      const notice = (event as CustomEvent<Notice>).detail;
      setNotices(previous => [...previous.filter(item => item.message !== notice.message), notice].slice(-3));
      timers.set(notice.id, setTimeout(() => {
        setNotices(previous => previous.filter(item => item.id !== notice.id));
        notice.close?.();
        timers.delete(notice.id);
      }, notice.tone === 'error' ? 10000 : 6000));
    };
    const dismiss = (event: Event) => {
      const id = (event as CustomEvent<string>).detail;
      clearTimeout(timers.get(id));
      timers.delete(id);
      setNotices(previous => previous.filter(item => item.id !== id));
    };
    window.addEventListener('cabinet-notice', receive);
    window.addEventListener('cabinet-notice-dismiss', dismiss);
    return () => { window.removeEventListener('cabinet-notice', receive); window.removeEventListener('cabinet-notice-dismiss', dismiss); timers.forEach(clearTimeout); };
  }, []);
  return createPortal(<div className="notice-stack" aria-label="Уведомления"><AnimatePresence initial={false}>{notices.map(notice => <motion.div layout={reduced ? false : 'position'} key={notice.id} className={`bottom-notice notice-${notice.tone}`} role={notice.tone === 'error' ? 'alert' : 'status'} initial={reduced ? false : { opacity: 0, y: 24, scale: 0.98 }} animate={{ opacity: 1, y: 0, scale: 1 }} exit={{ opacity: 0, y: 8, transition: { duration: reduced ? 0 : timing.exit } }} transition={reduced ? { duration: 0 } : springs.panel}><span className="notice-mark" aria-hidden="true">{notice.tone === 'success' ? '✓' : '!'}</span><span>{notice.message}</span><button type="button" aria-label="Закрыть уведомление" onClick={() => {setNotices(previous => previous.filter(item => item.id !== notice.id));notice.close?.();}}>×</button></motion.div>)}</AnimatePresence></div>, document.body);
}
export function BottomNotice({ message, onClose }: { message: string; onClose: () => void }) {
  const close = useRef(onClose);
  useEffect(() => { close.current = onClose; }, [onClose]);
  useEffect(() => {
    if (!message) return;
    const id = notify(message, 'error', () => close.current());
    return () => { window.dispatchEvent(new CustomEvent('cabinet-notice-dismiss', { detail: id })); };
  }, [message]);
  return null;
}
