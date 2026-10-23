import {chromium} from 'playwright';
import assert from 'node:assert/strict';
const b=await chromium.launch({channel:process.env.BROWSER_CHANNEL||'msedge'});
try {
 const c=await b.newContext();const p=await c.newPage();const errors=[];p.on('pageerror',e=>errors.push(e.message));
 const root=process.env.UX_BASE||'http://127.0.0.1:5175';
 await p.setViewportSize({width:390,height:844});await p.goto(root);await p.getByRole('heading',{name:'Вход в кредитный кабинет'}).waitFor();
 await p.keyboard.press('Tab');assert.equal(await p.getByRole('link',{name:'Перейти к содержимому'}).evaluate(e=>e===document.activeElement),true);
 await p.getByLabel('Логин',{exact:true}).fill('coffee');await p.getByLabel('Пароль',{exact:true}).fill('demo12345');await p.getByRole('button',{name:'Войти',exact:true}).click();await p.getByRole('heading',{name:'Обзор'}).waitFor();
 for(const width of [390,1366]) {await p.setViewportSize({width,height:900});assert.equal(await p.evaluate(()=>document.documentElement.scrollWidth>innerWidth),false);}
 const response=await c.request.post(root+'/api/applications',{headers:{'Idempotency-Key':crypto.randomUUID()},data:{inn:'0000100012',product:'working_capital',purpose:'purchase',representative_role:'director',amount_kop:250000000,term_months:24,bki_consent:true}});const app=await response.json();await c.request.post(root+'/api/applications/'+app.id+'/submit');await p.goto(root+'/client/applications/'+app.id);await p.getByRole('navigation',{name:'Этапы заявки'}).waitFor();
 assert.equal(await p.locator('[aria-current=step]').count(),1);
 for(const width of [390,1366]) {await p.setViewportSize({width,height:900});assert.equal(await p.evaluate(()=>document.documentElement.scrollWidth>innerWidth),false);if(process.env.UX_SHOTS)await p.screenshot({path:process.env.UX_SHOTS+'/card-'+width+'.png',fullPage:true});}
 assert.deepEqual(errors,[]);console.log('UX: keyboard skip link, progress, client list/card mobile and laptop, no page errors passed');
} finally {await b.close()}

