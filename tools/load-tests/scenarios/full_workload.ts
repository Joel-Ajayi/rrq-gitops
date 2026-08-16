/**
 * SCENARIO: Full Workload Mix Test
 * PRIMARY QUESTION ANSWERED: "How does the system perform under a realistic mix of all 8 production API endpoints?"
 * 
 * Target Traffic Pattern: Multi-endpoint arrival distribution matching production QPS ratios
 * Target Environment: Dev / Staging / Production
 * Outputs for slo-input.yaml: nominal_qps, avg_query_time_ms, c_s_squared, c_a_squared across all 8 endpoints
 */

import { Options, RampingArrivalRateScenario } from 'k6/options';
import { CONFIG } from '../lib/config.ts';
import { selectWalletPair, selectDepositWallet, WalletPair, DepositContext } from '../lib/data.ts';
import {
  submitTransfer,
  submitDeposit,
  getWalletBalance,
  getJobStatus,
  refreshToken,
  createMerchant,
  createWallet,
  replayDLQ,
  getActiveJwt,
} from '../lib/api.ts';

import exec from 'k6/execution';

export const options: Options = {
  scenarios: {
    full_workload_test: CONFIG.workloads.load as RampingArrivalRateScenario,
  },
  thresholds: CONFIG.thresholds.load,
};

export default function (): void {
  const rand: number = Math.random();
  const vu: number = exec.vu.idInTest;
  const iter: number = exec.scenario.iterationInTest;

  // Distribution matching slo-input.yaml endpoint nominal throughput ratios
  if (rand < 0.35) {
    // 35% Merchant Auth / Lookup (auth-token & merchant-lookup)
    const { apiKey, merchantIdx }: DepositContext = selectDepositWallet(vu, iter);
    refreshToken(merchantIdx, apiKey);
  } else if (rand < 0.65) {
    // 30% Create Transfer
    const { fromWallet, toWallet, jwt, apiKey, merchantIdx }: WalletPair = selectWalletPair(vu, iter);
    const activeJwt = getActiveJwt(merchantIdx, jwt);
    const res = submitTransfer(fromWallet, toWallet, 100, activeJwt);
    if (res.status === 401) {
      refreshToken(merchantIdx, apiKey);
    }
  } else if (rand < 0.75) {
    // 10% Get Balance
    const { walletId, jwt, apiKey, merchantIdx }: DepositContext = selectDepositWallet(vu, iter);
    const activeJwt = getActiveJwt(merchantIdx, jwt);
    const res = getWalletBalance(walletId, activeJwt);
    if (res.status === 401) {
      refreshToken(merchantIdx, apiKey);
    }
  } else if (rand < 0.85) {
    // 10% Get Job Status
    const { jwt, apiKey, merchantIdx }: DepositContext = selectDepositWallet(vu, iter);
    const activeJwt = getActiveJwt(merchantIdx, jwt);
    const res = getJobStatus('job_sample_id', activeJwt);
    if (res.status === 401) {
      refreshToken(merchantIdx, apiKey);
    }
  } else if (rand < 0.95) {
    // 10% Wallet Deposit (create-transfer with empty from_wallet)
    const { walletId, jwt, apiKey, merchantIdx }: DepositContext = selectDepositWallet(vu, iter);
    const activeJwt = getActiveJwt(merchantIdx, jwt);
    const res = submitDeposit(walletId, 500, activeJwt);
    if (res.status === 401) {
      refreshToken(merchantIdx, apiKey);
    }
  } else if (rand < 0.98) {
    // 3% Create Wallet
    const { jwt, apiKey, merchantIdx }: DepositContext = selectDepositWallet(vu, iter);
    const activeJwt = getActiveJwt(merchantIdx, jwt);
    const res = createWallet('NGN', activeJwt);
    if (res.status === 401) {
      refreshToken(merchantIdx, apiKey);
    }
  } else if (rand < 0.99) {
    // 1% Create Merchant
    createMerchant(`merchant_${vu}_${iter}`, 'https://example.com/webhook', 'secret_key');
  } else {
    // 1% Admin DLQ Replay
    const { jwt, apiKey, merchantIdx }: DepositContext = selectDepositWallet(vu, iter);
    const activeJwt = getActiveJwt(merchantIdx, jwt);
    replayDLQ(activeJwt);
  }
}
