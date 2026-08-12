import http, { RefinedResponse, ResponseType, Response } from 'k6/http';
import { check } from 'k6';
import { CONFIG } from './config.ts';
import {
  TransferSuccessCounter,
  TransferFailureCounter,
  TransferDurationTrend,
  TransferErrorRate,
  DepositSuccessCounter,
  DepositDurationTrend,
  BalanceDurationTrend,
} from './metrics.ts';

function uuidv4(): string {
  return 'xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx'.replace(/[xy]/g, function (c) {
    const r = (Math.random() * 16) | 0;
    const v = c === 'x' ? r : (r & 0x3) | 0x8;
    return v.toString(16);
  });
}

/**
 * Executes a POST /v1/transfers request (Synchronous 202 Ingress)
 * Payload matches apiv1.CreateTransferRequest proto specification.
 */
export function submitTransfer(
  fromWallet: string,
  toWallet: string,
  amount: number,
  jwt: string
): RefinedResponse<ResponseType> {
  const payload = JSON.stringify({
    from_wallet: fromWallet,
    to_wallet: toWallet,
    amount: amount,
    currency: 'NGN',
    reference: uuidv4(),
  });

  const params = {
    headers: {
      Authorization: `Bearer ${jwt}`,
      'Content-Type': 'application/json',
      'X-Idempotency-Key': uuidv4(),
    },
    tags: { name: 'POST /v1/transfers', endpoint: 'transfers' },
  };

  const res = http.post(`${CONFIG.baseUrl}/v1/transfers`, payload, params);

  const success = check(res, {
    'status is 202 or 200': (r: Response) => r.status === 202 || r.status === 200,
    'has jobId': (r: Response) => {
      try {
        const body = r.json() as Record<string, unknown>;
        return body !== null && (body.jobId !== undefined || body.job_id !== undefined);
      } catch {
        return false;
      }
    },
  });

  TransferDurationTrend.add(res.timings.duration);
  TransferErrorRate.add(!success);

  if (success) {
    TransferSuccessCounter.add(1);
  } else {
    TransferFailureCounter.add(1);
  }

  return res;
}

/**
 * Executes a GET /v1/balances request
 */
export function getWalletBalance(walletId: string, jwt: string): RefinedResponse<ResponseType> {
  const params = {
    headers: { Authorization: `Bearer ${jwt}` },
    tags: { name: 'GET /v1/balances', endpoint: 'balances' },
  };

  const res = http.get(`${CONFIG.baseUrl}/v1/balances?wallet_id=${walletId}`, params);

  check(res, {
    'status is 200': (r: Response) => r.status === 200,
  });

  BalanceDurationTrend.add(res.timings.duration);
  return res;
}

/**
 * Executes a deposit via POST /v1/transfers request (Synchronous 202 Ingress)
 * Payload matches apiv1.CreateTransferRequest proto specification but with an empty from_wallet.
 */
export function submitDeposit(walletId: string, amount: number, jwt: string): RefinedResponse<ResponseType> {
  const payload = JSON.stringify({
    from_wallet: '', // Empty from_wallet triggers the deposit route in core-api
    to_wallet: walletId,
    amount: amount,
    currency: 'NGN',
    reference: uuidv4(),
  });

  const params = {
    headers: {
      Authorization: `Bearer ${jwt}`,
      'Content-Type': 'application/json',
      'X-Idempotency-Key': uuidv4(),
    },
    tags: { name: 'POST /v1/transfers (Deposit)', endpoint: 'transfers_deposit' },
  };

  const res = http.post(`${CONFIG.baseUrl}/v1/transfers`, payload, params);

  const success = check(res, {
    'status is 202 or 200': (r: Response) => r.status === 202 || r.status === 200,
    'has jobId': (r: Response) => {
      try {
        const body = r.json() as Record<string, unknown>;
        return body !== null && (body.jobId !== undefined || body.job_id !== undefined);
      } catch {
        return false;
      }
    },
  });

  DepositDurationTrend.add(res.timings.duration);

  if (success) {
    DepositSuccessCounter.add(1);
  }

  return res;
}

/**
 * Executes a GET /v1/jobs/:id request
 */
export function getJobStatus(jobId: string, jwt: string): RefinedResponse<ResponseType> {
  const params = {
    headers: { Authorization: `Bearer ${jwt}` },
    tags: { name: 'GET /v1/jobs', endpoint: 'jobs' },
  };

  return http.get(`${CONFIG.baseUrl}/v1/jobs/${jobId}`, params);
}
