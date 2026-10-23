import assert from 'node:assert/strict';
const root='http://127.0.0.1:5175/api';
const login=await fetch(root+'/login',{method:'POST',body:JSON.stringify({login:'coffee',password:'demo12345'})});assert.equal(login.status,200);
const cookie=login.headers.get('set-cookie').split(';')[0];
const stream=await fetch(root+'/events',{headers:{cookie},signal:AbortSignal.timeout(5000)});assert.equal(stream.status,200);
const reader=stream.body.getReader();await reader.read();
const logout=await fetch(root+'/logout',{method:'POST',headers:{cookie}});assert.equal(logout.status,204);
assert.equal((await reader.read()).done,true);
assert.equal((await fetch(root+'/me',{headers:{cookie}})).status,401);
console.log('SSE: logout closes active stream and invalidates session passed');

