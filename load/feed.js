// Readers open their home feed, as fast as the API answers.
import http from 'k6/http';

export { handleSummary } from './summary.js';

export const options = {
  vus: Number(__ENV.VUS || 20),
  duration: '20s',
  summaryTrendStats: ['med', 'p(99)'],
};

export default function () {
  const id = 1 + Math.floor(Math.random() * 100000);
  http.get(`${__ENV.BASE_URL}/feed`, {
    headers: { Authorization: `Bearer seed-token-${id}` },
  });
}
