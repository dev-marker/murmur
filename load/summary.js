// Every load test in this course ends with the same four numbers.
export function handleSummary(data) {
  const m = data.metrics;
  const ms = (v) => `${v.toFixed(1)} ms`;
  const lines = [
    `requests per second  ${m.http_reqs.values.rate.toFixed(0)}`,
    `p50                  ${ms(m.http_req_duration.values.med)}`,
    `p99                  ${ms(m.http_req_duration.values['p(99)'])}`,
    `errors               ${(m.http_req_failed.values.rate * 100).toFixed(2)} %`,
  ];
  return { stdout: `\n${lines.join('\n')}\n\n` };
}
