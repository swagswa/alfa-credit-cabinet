import { useEffect, useState } from 'react';
import { Button } from '../motion';
import { api } from '../api/client';
import { fieldNames } from '../api/presentation';
import { FinancialsForm } from './FinancialsForm';
import { StatementForm } from './StatementForm';
import type { components } from '../api/schema.gen';
type Doc=components['schemas']['Document'];
export function DocumentDetails({id,inn,editable,bank,refresh,status}:{id:string;inn:string;editable:boolean;bank:boolean;refresh:()=>void;status:Doc["status"]}) {
 const [document,setDocument]=useState<Doc>();const [error,setError]=useState('');const [useExtracted,setUseExtracted]=useState(false);const [revision,setRevision]=useState(0);
 useEffect(()=>{let active=true;let timer:ReturnType<typeof setTimeout>;async function load(){try{const {data}=await api.GET('/documents/{id}',{params:{path:{id}}});if(!active)return;if(data){setDocument(data);setError('');if(data.status==='recognizing')timer=setTimeout(load,2000);}else setError('Не удалось загрузить данные документа.');}catch{if(active)setError('Нет связи с сервером документов.');}}void load();return()=>{active=false;clearTimeout(timer);};},[id,revision,status]);
 const initial=useExtracted?document?.extracted:undefined;
 return <div className="document-details">
 {error&&<p role="alert">{error}</p>}{error&&<Button size={40} onClick={()=>setRevision(v=>v+1)}>Повторить загрузку данных</Button>}
 {document?.status==='recognizing'&&<p role="status">Распознаём документ. Вы можете заполнить значения вручную, не дожидаясь результата.</p>}
 {document?.error&&<p role="alert">{document.error}. Доступен ручной ввод.</p>}
 {document?.validation_errors?.map((e,i)=><p role="alert" key={i}>{e.field}: {e.message}</p>)}
 {editable&&document?.extracted&&document.status!=='confirmed'&&<><p>Распознанные данные доступны. Загрузка значений заменит текущий ввод формы.</p><Button size={40} onClick={()=>setUseExtracted(true)} disabled={useExtracted}>Заполнить из документа</Button></>}
 {editable&&document?.status!=='confirmed'&&<div className={useExtracted?'extracted-fields':''} key={useExtracted?'extracted':'manual'}>
 {document?.kind==='financials'&&<FinancialsForm documentId={id} inn={inn} initial={initial?.financials} refresh={()=>{setRevision(v=>v+1);refresh();}}/>}
 {document?.kind==='statement'&&<StatementForm documentId={id} inn={inn} initial={initial?.statement} refresh={()=>{setRevision(v=>v+1);refresh();}}/>}</div>}
 {bank&&document&&<><p>Модель: {document.model??'Ручной ввод'}. Подтверждено: {document.confirmed_at?new Date(document.confirmed_at).toLocaleString('ru-RU'):'Нет'}.</p>
 <div className="table-scroll"><table><caption>Данные модели и клиента (суммы в копейках)</caption><thead><tr><th>Поле</th><th>Модель</th><th>Клиент</th></tr></thead><tbody>{Object.entries(document.confirmed?.statement??document.confirmed?.financials??document.extracted?.statement??document.extracted?.financials??{}).map(([key,value])=>{const raw=document.extracted?.statement??document.extracted?.financials;return <tr key={key} className={document.edited_fields?.includes(key)?'extracted':''}><th>{fieldNames[key] ?? key}{document.edited_fields?.includes(key)?' (изменено)':''}</th><td>{JSON.stringify(raw?.[key as keyof typeof raw])??'Нет'}</td><td>{JSON.stringify(value)}</td></tr>})}</tbody></table></div></>}
 </div>;
}
