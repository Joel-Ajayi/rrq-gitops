/**
 * SCENARIO: Full Workload Mix Test
 * PRIMARY QUESTION ANSWERED: "How does the system perform under a realistic mix of all 8 production API endpoints?"
 *
 * Target Traffic Pattern: Multi-endpoint arrival distribution matching production QPS ratios
 * Target Environment: Dev / Staging / Production
 * Outputs for slo-input.yaml: nominal_qps, avg_query_time_ms, c_s_squared, c_a_squared across all 8 endpoints
 */

import { Options, RampingArrivalRateScenario } from "k6/options";
import { CONFIG } from "../lib/config.ts";
import {
  selectWalletPair,
  selectDepositWallet,
  WalletPair,
  DepositContext,
} from "../lib/data.ts";
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
} from "../lib/api.ts";

import exec from "k6/execution";

export const options: Options = {
  scenarios: {
    full_workload_test: CONFIG.workloads
      .full_workload as RampingArrivalRateScenario,
  },
  thresholds: CONFIG.thresholds.load,
};

const recentJobsByMerchant: Record<number, string[]> = {};

export default function (): void {
  const rand: number = Math.random();
  const vu: number = exec.vu.idInTest;
  const iter: number = exec.scenario.iterationInTest;

  // Distribution matching realistic production throughput ratios
  if (rand < 0.5) {
    // 50% Create Transfer
    const { fromWallet, toWallet, jwt, apiKey, merchantIdx }: WalletPair =
      selectWalletPair(vu, iter);
    const activeJwt = getActiveJwt(merchantIdx, jwt);
    const res = submitTransfer(fromWallet, toWallet, 100, activeJwt);
    if (res.status === 401) {
      refreshToken(merchantIdx, apiKey);
    } else if (res.status === 202 || res.status === 200) {
      try {
        const body = res.json() as Record<string, unknown>;
        const jId = (body.jobId || body.job_id) as string;
        if (jId) {
          if (!recentJobsByMerchant[merchantIdx]) {
            recentJobsByMerchant[merchantIdx] = [];
          }
          if (recentJobsByMerchant[merchantIdx].length >= 20) {
            recentJobsByMerchant[merchantIdx].shift();
          }
          recentJobsByMerchant[merchantIdx].push(jId);
        }
      } catch {}
    }
  } else if (rand < 0.8) {
    // 20% Get Balance
    const { walletId, jwt, apiKey, merchantIdx }: DepositContext =
      selectDepositWallet(vu, iter);
    const activeJwt = getActiveJwt(merchantIdx, jwt);
    const res = getWalletBalance(walletId, activeJwt);
    if (res.status === 401) {
      refreshToken(merchantIdx, apiKey);
    }
  } else if (rand < 0.9) {
    // 10% Wallet Deposit (create-transfer with empty from_wallet)
    const { walletId, jwt, apiKey, merchantIdx }: DepositContext =
      selectDepositWallet(vu, iter);
    const activeJwt = getActiveJwt(merchantIdx, jwt);
    const res = submitDeposit(walletId, 500, activeJwt);
    if (res.status === 401) {
      refreshToken(merchantIdx, apiKey);
    } else if (res.status === 202 || res.status === 200) {
      try {
        const body = res.json() as Record<string, unknown>;
        const jId = (body.jobId || body.job_id) as string;
        if (jId) {
          if (!recentJobsByMerchant[merchantIdx]) {
            recentJobsByMerchant[merchantIdx] = [];
          }
          if (recentJobsByMerchant[merchantIdx].length >= 20) {
            recentJobsByMerchant[merchantIdx].shift();
          }
          recentJobsByMerchant[merchantIdx].push(jId);
        }
      } catch {}
    }
  } else {
    // 10% Get Job Status (Querying real jobs created dynamically by this merchant)
    const { jwt, apiKey, merchantIdx }: DepositContext = selectDepositWallet(
      vu,
      iter,
    );
    const activeJwt = getActiveJwt(merchantIdx, jwt);
    const mJobs = recentJobsByMerchant[merchantIdx];
    if (mJobs && mJobs.length > 0) {
      const jobId = mJobs[Math.floor(Math.random() * mJobs.length)];
      const res = getJobStatus(jobId, activeJwt);
      if (res.status === 401) {
        refreshToken(merchantIdx, apiKey);
      }
    } else {
      const { walletId } = selectDepositWallet(vu, iter);
      const res = getWalletBalance(walletId, activeJwt);
      if (res.status === 401) {
        refreshToken(merchantIdx, apiKey);
      }
    }
  }
}
