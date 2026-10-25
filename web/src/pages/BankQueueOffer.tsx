import { useEffect, useState } from 'react';
import { api, settle } from '../api/client';
import { money } from '../api/presentation';
import type { components } from '../api/schema.gen';
import { useCabinetCache } from '../CabinetCache';

type Summary = components['schemas']['ApplicationSummary'];
type Offer = components['schemas']['Offer'];

export function BankQueueOffer({ item }: { item: Summary }) {
  const cache = useCabinetCache();
  const cacheKey = `bank-queue-offer:${item.id}:${item.updated_at}`;
  const [offer, setOffer] = useState<Offer | undefined>(() => cache.get<Offer>(cacheKey));
  const [failed, setFailed] = useState(false);
  useEffect(() => {
    if (cache.get<Offer>(cacheKey)) return;
    return settle(
      cache.request(cacheKey, () => api.GET('/applications/{id}', { params: { path: { id: item.id } } })),
      ({ data }) => {
        if (data?.offer) {
          cache.set(cacheKey, data.offer);
          setOffer(data.offer);
        } else setFailed(true);
      },
      () => setFailed(true),
    );
  }, [cache, cacheKey, item.id]);
  return (
    <div className="bank-queue-offer">
      <small>Предложено клиенту</small>
      {offer ? (
        <>
          <strong className="application-amount">{money(offer.amount_kop)}</strong>
          <span>{offer.term_months} мес.</span>
        </>
      ) : (
        <span>{failed ? 'Не удалось загрузить условия. Откройте карточку заявки.' : 'Загружаем предложение…'}</span>
      )}
      <small>
        Запрошено: {money(item.amount_kop)} · {item.term_months} мес.
      </small>
    </div>
  );
}
