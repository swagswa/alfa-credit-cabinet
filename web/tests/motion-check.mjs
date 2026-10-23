import {chromium} from 'playwright';
import assert from 'node:assert/strict';
const browser=await chromium.launch({channel:process.env.BROWSER_CHANNEL||'msedge'});
try {
const c=await browser.newContext();const p=await c.newPage();await p.goto('http://127.0.0.1:5175');await p.getByLabel('Логин',{exact:true}).fill('coffee');await p.getByLabel('Пароль',{exact:true}).fill('demo12345');await p.getByRole('button',{name:'Войти',exact:true}).click();await p.getByRole('heading',{name:'Обзор',exact:true}).waitFor();
await p.emulateMedia({reducedMotion:'reduce'});await p.getByRole('navigation',{name:'Разделы кабинета'}).getByRole('link',{name:'Новая заявка',exact:true}).click();await p.getByLabel('ИНН компании',{exact:true}).waitFor();
assert.equal(await p.locator('button').first().evaluate(el=>getComputedStyle(el).transitionDuration),'0s');
assert.equal(await p.evaluate(()=>document.getAnimations().filter(a=>a.playState==='running').length),0);
await p.setViewportSize({width:390,height:844});assert.equal(await p.evaluate(()=>document.documentElement.scrollWidth>innerWidth),false);
console.log('Motion: reduced-motion stops animations, touch viewport has no overflow passed');
}finally{await browser.close()}
