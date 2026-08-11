import http from 'k6/http';
import exec from 'k6/execution';
import { check } from 'k6';
import { SharedArray } from 'k6/data';

const planDoc = JSON.parse(open(__ENV.EMAC_STAGE_PLAN));
const requests = new SharedArray('registered-stage-plan', () => planDoc.requests);
const frontend = (__ENV.FRONTEND_URL || 'http://frontend:8080').replace(/\/$/, '');
const policy = (__ENV.POLICY_URL || 'http://checkout-policy:8080').replace(/\/$/, '');
const rate = Number(__ENV.EMAC_RATE || 5);
const duration = __ENV.EMAC_DURATION || `${Math.ceil(requests.length / rate)}s`;

export const options = {
  scenarios: {
    registered_stage: {
      executor: 'constant-arrival-rate',
      rate,
      timeUnit: '1s',
      duration,
      preAllocatedVUs: Number(__ENV.EMAC_PREALLOCATED_VUS || 50),
      maxVUs: Number(__ENV.EMAC_MAX_VUS || 200),
      gracefulStop: '30s',
    },
  },
  systemTags: ['status', 'method', 'url', 'name', 'scenario'],
};

const products = ['OLJCESPC7Z', '66VCHSJNUP'];

function correctItems(items) {
  if (!Array.isArray(items) || items.length !== 2) return false;
  const quantities = Object.fromEntries(products.map((id) => [id, 0]));
  for (const value of items) {
    const item = value?.item;
    if (!(item?.productId in quantities)) return false;
    quantities[item.productId] += item.quantity;
  }
  return products.every((id) => quantities[id] === 1);
}

function traceparent(requestId) {
  const trace = requestId.replaceAll('-', '').slice(0, 32);
  return `00-${trace}-${trace.slice(0, 16)}-00`;
}

function headers(v, phase) {
  return {
    'Content-Type': 'application/json',
    'X-Emac-Run-Id': planDoc.run_id,
    'X-Emac-Stage-Id': planDoc.stage_id,
    'X-Emac-Request-Id': v.request_id,
    'X-Emac-Rollout-Key': v.rollout_key,
    'X-Emac-International': String(v.international),
    'X-Emac-Phase': phase,
  };
}

export default function () {
  const i = exec.scenario.iterationInTest;
  if (i >= requests.length) return;
  const v = requests[i];

  // Cart population is a separate, explicitly unsampled setup trace. The
  // policy-root journey begins only at the checkout POST below.
  for (const productId of products) {
    const setupHeaders = { ...headers(v, 'setup'), traceparent: traceparent(v.request_id) };
    const response = http.post(`${frontend}/api/cart?currencyCode=${v.currency}`,
      JSON.stringify({ userId: v.user_id, item: { productId, quantity: 1 } }),
      { headers: setupHeaders, tags: { emac_phase: 'setup', emac_operation: 'cart_setup' } });
    check(response, { 'cart setup 200': (r) => r.status === 200 });
  }

  const address = v.international
    ? { streetAddress: '1200 Maple Street', city: 'Toronto', state: 'ON', country: 'Canada', zipCode: 'M4B 1B3' }
    : { streetAddress: '1600 Pennsylvania Avenue NW', city: 'Washington', state: 'DC', country: 'United States', zipCode: '20500' };
  const checkout = {
    userId: v.user_id,
    userCurrency: v.currency,
    address,
    email: 'emac@example.invalid',
    creditCard: { creditCardNumber: '4432801561520454', creditCardCvv: 672, creditCardExpirationYear: 2039, creditCardExpirationMonth: 1 },
  };
  const response = http.post(`${policy}/api/checkout?currencyCode=${v.currency}`,
    JSON.stringify(checkout),
    { headers: headers(v, v.phase), tags: { emac_phase: v.phase, emac_branch: v.branch, emac_operation: 'policy_root' } });
	let body;
	let schemaCorrect = false;
	try {
		body = response.json();
		schemaCorrect = Boolean(body.orderId && body.shippingTrackingId && correctItems(body.items));
	} catch (_) {
		schemaCorrect = false;
	}
	const rootCorrect = response.status === 200 && schemaCorrect;
  check(response, {
    'checkout status 200': (r) => r.status === 200,
		'checkout schema correct': () => schemaCorrect,
  });
	if (v.phase === 'measured' || v.phase === 'oracle') {
		console.log(`EMAC_ORACLE ${JSON.stringify({
			request_id: v.request_id,
			phase: v.phase,
			branch: v.branch,
			correct: rootCorrect,
			duration_ms: response.timings.duration,
		})}`);
	}
}
