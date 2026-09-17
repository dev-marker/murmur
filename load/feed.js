// Readers open their home feed, as fast as the API answers.
import http from 'k6/http';

export { handleSummary } from './summary.js';

export const options = {
  vus: Number(__ENV.VUS || 20),
  duration: '20s',
  summaryTrendStats: ['med', 'p(99)'],
};

// How many different users read during the test. Everybody else is offline.
const readers = Number(__ENV.READERS || 100000);

export default function () {
  const id = 1 + Math.floor(Math.random() * readers);
  http.get(`${__ENV.BASE_URL}/feed`, {
    headers: { Authorization: `Bearer seed-token-${id}` },
  });
}
