// Authors post, as fast as the API answers.
import http from 'k6/http';

export { handleSummary } from './summary.js';

export const options = {
  vus: Number(__ENV.VUS || 20),
  duration: '20s',
  summaryTrendStats: ['med', 'p(99)'],
};

// Who posts: the AUTHORS biggest accounts. Left unset, it's anybody.
const authors = Number(__ENV.AUTHORS || 100000);
// FIRST is the biggest of them. FIRST=119 starts just under the line.
const first = Number(__ENV.FIRST || 1);

export default function () {
  const id = first + Math.floor(Math.random() * authors);
  http.post(`${__ENV.BASE_URL}/posts`, JSON.stringify({ body: 'a post from the load test' }), {
    headers: { Authorization: `Bearer seed-token-${id}`, 'Content-Type': 'application/json' },
  });
}
