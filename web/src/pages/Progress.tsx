import { motion, useMotionDisabled } from '../motion';
import type { components } from '../api/schema.gen';
export function Progress({ card, selected, onSelect }: { card: components['schemas']['ApplicationView']; selected?: number; onSelect?: (step: number) => void }) {
 const reduced = useMotionDisabled();
 const steps = ['Заполнение', 'Рассмотрение', 'Предложение банка', 'Документы и подпись', 'Результат'];
 const index = card.status === 'draft' ? 0 : ['submitted','checking','need_info','scoring','manual_review'].includes(card.status) ? 1 : ['approved','counter_offer'].includes(card.status) ? 2 : ['accepted','signing'].includes(card.status) ? 3 : 4;
 const active = selected ?? index;
 return <nav aria-label="Этапы заявки"><ol className={`progress${onSelect ? ' progress-browsable' : ''}`}>{steps.map((step,i)=>{
 const content = <><span aria-hidden="true">{i===active && <motion.i className="step-selection" layoutId="application-stage" transition={reduced ? {duration:0} : undefined}/>}<b>{i+1}</b></span>{step}</>;
 return <li key={step} aria-current={!onSelect && i===active?'step':undefined} className={i<index && !['rejected','expired','cancelled_by_client'].includes(card.status)?'complete':''}>{onSelect ? <button type="button" aria-label={`${i+1}. ${step}`} aria-current={i===active?'step':undefined} aria-controls="journey-view" onClick={()=>onSelect(i)}>{content}</button> : content}</li>;
 })}</ol>{!onSelect && <p className="hint">{card.status==='need_info'?'Сейчас нужен ваш ответ. Список документов и срок указаны ниже.':card.status==='manual_review'?'Заявку рассматривает сотрудник банка. Решение появится в кабинете.':card.status==='rejected'?'Рассмотрение завершилось отказом. Прочитайте объяснение банка ниже.':card.status==='cancelled_by_client'?'Процесс остановлен по вашей инициативе.':card.status==='expired'?'Процесс остановлен: срок ответа истёк.':'Текущий этап выделен. Следующее действие показано в карточке.'}</p>}</nav>;
}


