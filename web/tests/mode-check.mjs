import assert from 'node:assert/strict';
const response = await fetch('http://127.0.0.1:5175/api/health');
assert.equal(response.headers.get('X-Mock-Api'), null);
assert.ok(response.status !== 200 || !(await response.text()).includes('contract-examples'));
console.log('Ordinary development mode: contract mock is disabled (backend availability not asserted)');

