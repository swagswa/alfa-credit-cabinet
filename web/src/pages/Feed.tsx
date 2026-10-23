import { Icon } from '../ui/Workspace';
import { useCabinetCache } from '../CabinetCache';
import { motion, AnimatePresence, Button, ListItem } from '../motion';
import { useEffect, useState } from 'react';
import { Link } from 'react-router';
import { api } from '../api/client';
import type { components } from '../api/schema.gen';
export function Feed() {
 const cache = useCabinetCache();
 const cached = cache.get<components['schemas']['FeedItem'][]>('feed');
 const [items,setItems]=useState<components['schemas']['FeedItem'][]>(cached ?? []);
 const [error,setError]=useState('');const [loading,setLoading]=useState(!cached);const [revision,setRevision]=useState(0);const [refreshing,setRefreshing]=useState(false);
 useEffect(()=>{let active=true;let serial=0;async function load(){const request=++serial;try{const {data}=await api.GET('/feed',{params:{query:{limit:30}}});if(!active||request!==serial)return;if(data){cache.set('feed',data);setItems(data);setError('');}else setError('Не удалось загрузить события. Остальные разделы доступны.');}catch{if(active&&request===serial)setError('События временно недоступны. Остальные разделы доступны.');}finally{if(active&&request===serial){setLoading(false);setRefreshing(false);}}}
 void load();window.addEventListener('feed-refresh',load);return()=>{active=false;window.removeEventListener('feed-refresh',load);};},[revision,cache]);
 const events = items.filter(item => item.kind === 'event');
 const [expanded, setExpanded] = useState(false);
 const visible = expanded ? events : events.slice(0, 5);
 return <section className="feed dashboard-feed" aria-busy={loading} aria-labelledby="events-heading">
 <div className="dashboard-section-heading"><div><h2 id="events-heading">События</h2><p>Последние изменения по вашим заявкам</p></div><Button className="events-refresh" size={40} aria-label="Обновить события" disabled={refreshing} aria-busy={refreshing} onClick={()=>{if(refreshing)return;setRefreshing(true);setRevision(v=>v+1);}} view="text"><span className="events-refresh-rotation" style={{transform:`rotate(${revision*360}deg)`}}><Icon name="refresh" /></span></Button></div>
 {loading&&<p role="status">Загружаем события…</p>}{error&&<p role="alert">{error}</p>}
 {!loading&&!error&&!events.length&&<div className="events-empty"><span aria-hidden="true">○</span><p>Пока нет событий</p><small>Здесь появятся решения банка, запросы документов и обновления заявок.</small></div>}
 <motion.ul className="events-list"><AnimatePresence initial={false}>{visible.map(item=><ListItem layout={false} className="event-row" key={item.id}><div className="event-icon" aria-hidden="true"><svg width="20" height="20" viewBox="0 0 24 24" fill="none"><path d="M6 9a6 6 0 0 1 12 0v5l2 3H4l2-3V9Zm4 11h4" stroke="currentColor" strokeWidth="1.6" strokeLinecap="round" strokeLinejoin="round"/></svg></div><div className="event-content"><h3>{item.title}</h3><p>{item.text}</p>{item.application_id&&<Link to={`/client/applications/${encodeURIComponent(item.application_id)}`}>Открыть заявку <span aria-hidden="true">→</span></Link>}</div><time dateTime={item.created_at}>{new Date(item.created_at).toLocaleString('ru-RU',{day:'numeric',month:'short',hour:'2-digit',minute:'2-digit'})}</time></ListItem>)}</AnimatePresence></motion.ul>
 {events.length>5&&<Button size={40} view="text" onClick={()=>setExpanded(value=>!value)}>{expanded?'Свернуть':'Показать ещё события'}</Button>}
 </section>;
}
