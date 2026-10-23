import {chromium} from 'playwright';
import assert from 'node:assert/strict';
const b=await chromium.launch({channel:process.env.BROWSER_CHANNEL || 'msedge'});
try {
 const c=await b.newContext(),p=await c.newPage();await p.goto('http://127.0.0.1:5175');await p.getByLabel('Логин',{exact:true}).fill('logistics');await p.getByLabel('Пароль',{exact:true}).fill('demo12345');await p.getByRole('button',{name:'Войти',exact:true}).click();await p.getByRole('heading',{name:'Обзор'}).waitFor();
 const base='http://127.0.0.1:5175/api';const r=await c.request.post(base+'/applications',{headers:{'Idempotency-Key':crypto.randomUUID()},data:{inn:'0000100012',representative_role:'director',product:'working_capital',purpose:'purchase',amount_kop:250000000,term_months:24,bki_consent:true}});const app=await r.json();await c.request.post(base+'/applications/'+app.id+'/submit');await p.goto('http://127.0.0.1:5175/client/applications/'+app.id);
 await p.getByRole('button',{name:'Принять условия',exact:true}).click();await p.getByRole('button',{name:'Получить новый код',exact:true}).click();
  for(let i=0;i<3;i++) {
   await p.getByLabel('Код подписания',{exact:true}).fill('000000');
   await p.getByRole('button',{name:'Подписать',exact:true}).click();
   await p.getByRole('alert').filter({hasText:'Неверный код'}).waitFor();
   if(i<2) await p.getByLabel('Код подписания',{exact:true}).filter({visible:true}).waitFor();
 }
 await p.getByLabel('Код подписания',{exact:true}).waitFor({state:'detached'});
 await p.getByRole('button',{name:'Получить новый код',exact:true}).click();
 const text=await p.getByText(/^Код: /).textContent();const code=text.match(/Код: (\d{6})/)[1];
 await p.getByLabel('Код подписания',{exact:true}).fill(code);await p.getByRole('button',{name:'Подписать',exact:true}).click();await p.getByText('Деньги на счёте. Выдача смоделирована, реального перевода нет.',{exact:true}).waitFor();
 assert.equal(await p.getByRole('button',{name:'Отменить заявку',exact:true}).count(),0);console.log('Browser: counter-offer, 3 invalid codes, new code, issued passed');
}finally{await b.close()}



