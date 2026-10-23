import { chromium } from 'playwright';
import assert from 'node:assert/strict';
const b = await chromium.launch({ channel: process.env.BROWSER_CHANNEL || 'msedge' });
try {
 const c=await b.newContext(),p=await c.newPage();
 await p.goto('http://127.0.0.1:5175/bank'); await p.getByRole('heading',{name:'Вход в кредитный кабинет'}).waitFor();
 await p.getByLabel('Логин',{exact:true}).fill('coffee'); await p.getByLabel('Пароль',{exact:true}).fill('wrong'); await p.getByRole('button',{name:'Войти',exact:true}).click(); await p.getByRole('alert').filter({hasText:'Неверный логин'}).waitFor();
 await p.getByLabel('Пароль',{exact:true}).fill('demo12345'); await p.getByRole('button',{name:'Войти',exact:true}).click(); await p.getByRole('heading',{name:'Обзор'}).waitFor();
 await p.goto('http://127.0.0.1:5175/bank');await p.getByRole('heading',{name:'Обзор'}).waitFor();assert.match(p.url(),/client/);
 await c.clearCookies();await p.evaluate(()=>window.dispatchEvent(new Event('feed-refresh')));await p.getByRole('heading',{name:'Вход в кредитный кабинет'}).waitFor(); assert.equal(await p.getByRole('button',{name:'Выйти',exact:true}).count(),0);
 await p.route('**/api/login',route=>route.fulfill({status:429,contentType:'application/problem+json',body:JSON.stringify({title:'Лимит',status:429})}));await p.getByLabel('Логин',{exact:true}).fill('coffee'); await p.getByLabel('Пароль',{exact:true}).fill('demo12345');await p.getByRole('button',{name:'Войти',exact:true}).click();await p.getByRole('alert').filter({hasText:'Слишком много попыток'}).waitFor();
 console.log('Browser: role guard, invalid login, lost session clears cabinet, 429 passed');
} finally {await b.close()}


