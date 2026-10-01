// One client hammers the API: a single account posts as fast as the API answers.
import http from 'k6/http';
import { Counter } from 'k6/metrics';

import { handleSummary as fourNumbers } from './summary.js';

export const options = {
  vus: Number(__ENV.VUS || 100), // one client, and this many connections
  duration: '20s',
  summaryTrendStats: ['med', 'p(99)'],
};

const made = new Counter('posts_made'); // 201: the post is in
const refused = new Counter('refused'); // 429: too many requests

const post = JSON.stringify({ body: 'FREE FOLLOWERS, CLICK HERE' });
const headers = { Authorization: 'Bearer seed-token-16726', 'Content-Type': 'application/json' };

export default function () {
  const res = http.post(`${__ENV.BASE_URL}/posts`, post, { headers });
  made.add(res.status === 201 ? 1 : 0);
  refused.add(res.status === 429 ? 1 : 0);
}

// The usual four numbers, and then what this one account got out of it.
export function handleSummary(data) {
  const out = fourNumbers(data);
  const count = (name) => data.metrics[name].values.count;
  out.stdout += `posts made           ${count('posts_made')}\n`;
  out.stdout += `refused with a 429   ${count('refused')}\n\n`;
  return out;
}
