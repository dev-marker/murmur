// A realistic mix: thousands of people online, and about a hundred reads for every write.
import http from 'k6/http';
import exec from 'k6/execution';

const BASE = __ENV.BASE_URL;
// How many different people are online during the test. Everybody else is away.
const ONLINE = 20000;
const TIME = __ENV.TIME || '30s';

// Open: RATE requests start every second, whether the last ones were answered or not.
const open = {
  executor: 'constant-arrival-rate',
  rate: Number(__ENV.RATE || 500),
  timeUnit: '1s',
  duration: TIME,
  preAllocatedVUs: 2000,
};
// Closed, like every test before this one: VUS connections, and each of them sends
// its next request when the last one was answered.
const closed = { executor: 'constant-vus', vus: Number(__ENV.VUS), duration: TIME };

const scenarios = { mix: __ENV.VUS ? closed : open };
// SPIKE=500: twenty seconds in, 500 posts a second arrive on top of the mix, for ten seconds.
if (__ENV.SPIKE) {
  scenarios.spike = {
    executor: 'constant-arrival-rate',
    exec: 'post',
    startTime: '20s',
    rate: Number(__ENV.SPIKE),
    timeUnit: '1s',
    duration: '10s',
    preAllocatedVUs: 500,
  };
}
// STAR=400: twenty seconds in, user 1 posts. Then a crowd likes that post, 400 times
// a second, for fifteen seconds.
if (__ENV.STAR) {
  scenarios.star = { executor: 'per-vu-iterations', exec: 'star', startTime: '20s', vus: 1, iterations: 1 };
  scenarios.crowd = {
    executor: 'constant-arrival-rate',
    exec: 'fan',
    startTime: '22s',
    rate: Number(__ENV.STAR),
    timeUnit: '1s',
    duration: '15s',
    preAllocatedVUs: 500,
  };
}

export const options = { scenarios, summaryTrendStats: ['med', 'p(99)'] };

// Ids 2 to 20,001. User 1 is the celebrity: it stays out of the crowd, and it only posts when STAR says so.
const someone = () => 2 + Math.floor(Math.random() * ONLINE);
const as = (id) => ({ Authorization: `Bearer seed-token-${id}` });
const json = (id) => ({ ...as(id), 'Content-Type': 'application/json' });

// Out of a thousand requests, 990 read and 10 write.
const mix = [
  [600, feed],
  [250, unread],
  [100, profile],
  [40, inbox],
  [5, like],
  [3, post],
  [2, follow],
];

export default function () {
  let n = Math.random() * 1000;
  for (const [share, request] of mix) {
    if ((n -= share) < 0) return request();
  }
}

function feed() {
  http.get(`${BASE}/feed`, { headers: as(someone()) });
}

function unread() {
  http.get(`${BASE}/notifications/unread`, { headers: as(someone()) });
}

// A tag gives every address with an id in it one name, or k6 keeps its numbers per URL.
function profile() {
  http.get(`${BASE}/users/user${someone()}/posts`, { tags: { name: 'profile' } });
}

function inbox() {
  http.get(`${BASE}/notifications`, { headers: as(someone()) });
}

function like() {
  const id = 1 + Math.floor(Math.random() * 2000000);
  http.post(`${BASE}/posts/${id}/like`, null, { headers: as(someone()), tags: { name: 'like' } });
}

export function post() {
  const body = JSON.stringify({ body: 'a post from the mix' });
  http.post(`${BASE}/posts`, body, { headers: json(someone()) });
}

function follow() {
  const other = `user${1 + Math.floor(Math.random() * 100000)}`;
  http.post(`${BASE}/users/${other}/follow`, null, { headers: as(someone()), tags: { name: 'follow' } });
}

// The biggest account posts once.
export function star() {
  http.post(`${BASE}/posts`, JSON.stringify({ body: 'big news' }), { headers: json(1) });
}

// A fan opens user 1's page, and likes the newest post on it. Every fan is a different person.
export function fan() {
  const page = http.get(`${BASE}/users/user1/posts`, { tags: { name: 'profile' } });
  const newest = page.json('posts.0.id');
  const headers = as(2 + exec.scenario.iterationInTest);
  http.post(`${BASE}/posts/${newest}/like`, null, { headers, tags: { name: 'like' } });
}

// The mix ends with one line, and not with five. So several runs make a table.
export function handleSummary(data) {
  const m = data.metrics;
  const ms = (stat) => `${m.http_req_duration.values[stat].toFixed(1)} ms`.padStart(9);
  const n = (v, width) => String(v).padStart(width);
  const dropped = m.dropped_iterations ? m.dropped_iterations.values.count : 0;
  const line = [
    `asked ${n(__ENV.VUS ? '-' : __ENV.RATE || 500, 4)}`,
    `done ${n(m.http_reqs.values.rate.toFixed(0), 4)}`,
    `p50 ${ms('med')}`,
    `p99 ${ms('p(99)')}`,
    `failed ${n(m.http_req_failed.values.passes, 5)}`,
    `dropped ${n(dropped, 5)}`,
  ];
  return { stdout: `${line.join('   ')}\n` };
}
