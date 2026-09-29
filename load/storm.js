// A posting storm: posts arrive at a steady rate, whether the API keeps up or not.
import http from 'k6/http';

export { handleSummary } from './summary.js';

export const options = {
  scenarios: {
    storm: {
      executor: 'constant-arrival-rate',
      rate: Number(__ENV.RATE || 400), // new posts per second
      timeUnit: '1s',
      duration: '20s',
      preAllocatedVUs: 100,
      maxVUs: 400,
    },
  },
  summaryTrendStats: ['med', 'p(99)'],
};

export default function () {
  const id = 1 + Math.floor(Math.random() * 100000);
  http.post(`${__ENV.BASE_URL}/posts`, JSON.stringify({ body: 'a post from the storm' }), {
    headers: { Authorization: `Bearer seed-token-${id}`, 'Content-Type': 'application/json' },
  });
}
