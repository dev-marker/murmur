// A like storm: a crowd of different people, and every one of them likes once.
import http from 'k6/http';
import exec from 'k6/execution';

export { handleSummary } from './summary.js';

export const options = {
  vus: Number(__ENV.VUS || 20),
  duration: '20s',
  summaryTrendStats: ['med', 'p(99)'],
};

// POST is the one post that everybody likes. Left unset, everybody likes a different post.
const post = Number(__ENV.POST || 0);

export default function () {
  // A new user every time. Nobody likes twice, so nobody's own rate limit is in the way.
  const user = 1 + (exec.scenario.iterationInTest % 100000);
  const id = post || 1 + Math.floor(Math.random() * 2000000);
  http.post(`${__ENV.BASE_URL}/posts/${id}/like`, null, {
    headers: { Authorization: `Bearer seed-token-${user}` },
    tags: { name: 'like' }, // one name for every post, or k6 keeps its numbers per URL
  });
}
