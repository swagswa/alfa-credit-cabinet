import { createContext, useContext, useState, type ReactNode } from 'react';
import type { components } from './api/schema.gen';
import { applicationGroups, defaultApplicationKey, type ApplicationGroup, type DashboardSnapshot } from './api/dashboard';

class CabinetCache {
  private values = new Map<string, unknown>();
  get<T>(key: string): T | undefined { return this.values.get(key) as T | undefined; }
  set<T>(key: string, value: T) { this.values.set(key, value); }
  rememberApplication(application: components['schemas']['ApplicationView'], created = false) {
    const dashboard = this.get<DashboardSnapshot>('dashboard');
    const previous = dashboard?.page.items.find(item => item.id === application.id) ?? dashboard?.attention.find(item => item.id === application.id);
    for (const bank of [false, true]) {
      const key = JSON.stringify(['applications', bank, 0, null, null, null, '-created_at']);
      const page = this.get<components['schemas']['ApplicationPage']>(key);
      if (!page) continue;
      const exists = page.items.some(item => item.id === application.id);
      if (!exists && !created) continue;
      this.set(key, {
        total: page.total + (exists ? 0 : 1),
        items: exists ? page.items.map(item => item.id === application.id ? application : item) : [application, ...page.items].slice(0, 20),
      });
    }
    if (dashboard && (created || previous)) {
      const counts = { ...dashboard.counts };
      for (const group of Object.keys(applicationGroups) as ApplicationGroup[]) {
        const statuses = applicationGroups[group].statuses;
        if (created || previous?.status !== application.status) {
          if (!created && statuses.some(status => status === previous?.status)) counts[group] = Math.max(0, counts[group] - 1);
          if (statuses.some(status => status === application.status)) counts[group]++;
        }
      }
      const attention = dashboard.attention.filter(item => item.id !== application.id);
      if (['need_info', 'approved', 'counter_offer', 'accepted', 'signing'].includes(application.status)) attention.push(application);
      attention.sort((left, right) => (left.due_at ?? '9999').localeCompare(right.due_at ?? '9999'));
      this.set('dashboard', { ...dashboard, counts, page: this.get(defaultApplicationKey) ?? dashboard.page, attention: attention.slice(0, 3) });
    }
  }
}
const Context = createContext<CabinetCache | null>(null);
// The provider is keyed by authenticated user. Nothing survives logout or account changes.
export function CabinetCacheProvider({ children }: { children: ReactNode }) {
  const [cache] = useState(() => new CabinetCache());
  return <Context.Provider value={cache}>{children}</Context.Provider>;
}
export function useCabinetCache() {
  const cache = useContext(Context);
  if (!cache) throw new Error('CabinetCacheProvider is required');
  return cache;
}
