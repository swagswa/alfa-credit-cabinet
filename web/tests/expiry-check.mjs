import {chromium} from 'playwright';
import assert from 'node:assert/strict';
const b=await chromium.launch({channel:process.env.BROWSER_CHANNEL || 'msedge'});
try {
 const c=await b.newContext(),p=await c.newPage(); await p.clock.install();await p.goto('http://127.0.0.1:5175');await p.getByLabel('Логин',{exact:true}).fill('coffee');await p.getByLabel('Пароль',{exact:true}).fill('demo12345');await p.getByRole('button',{name:'Войти',exact:true}).click();await p.getByRole('heading',{name:'Обзор'}).waitFor();
 const base='http://127.0.0.1:5175/api';const r=await c.request.post(base+'/applications',{headers:{'Idempotency-Key':crypto.randomUUID()},data:{inn:'0000100012',representative_role:'director',product:'working_capital',purpose:'purchase',amount_kop:250000000,term_months:24,bki_consent:true}});const app=await r.json();await c.request.post(base+'/applications/'+app.id+'/submit');await p.goto('http://127.0.0.1:5175/client/applications/'+app.id);
 await p.getByRole('button',{name:'Принять условия',exact:true}).click();await p.getByRole('button',{name:'Получить новый код',exact:true}).click();
 await p.clock.fastForward(181000);
 await p.getByRole('alert').filter({hasText:'Срок кода истёк'}).waitFor();
 assert.equal(await p.getByRole('button',{name:'Подписать',exact:true}).count(),0);
 const second=await c.newPage(); await second.clock.install();
 const rr=await c.request.post(base+'/applications',{headers:{'Idempotency-Key':crypto.randomUUID()},data:{inn:'0000100012',representative_role:'director',product:'working_capital',purpose:'purchase',amount_kop:250000000,term_months:24,bki_consent:true}});const next=await rr.json();await c.request.post(base+'/applications/'+next.id+'/submit');await second.goto('http://127.0.0.1:5175/client/applications/'+next.id);
 await second.getByRole('button',{name:'Принять условия',exact:true}).waitFor();
 await second.clock.fastForward(181000);
 await second.getByRole('alert').filter({hasText:'Срок предложения истёк'}).waitFor();
 assert.equal(await second.getByRole('button',{name:'Принять условия',exact:true}).count(),0);
 console.log('Browser clock: expired code and offer actions hidden, explanations shown');
}finally{await b.close()}


