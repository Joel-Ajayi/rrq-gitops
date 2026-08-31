/**
 * SCENARIO: Stress Test (Peak Capacity & Backpressure)
 * PRIMARY QUESTION ANSWERED: "Can horizontal autoscaling (KEDA/HPA) and backpressure mechanisms handle projected peak traffic?"
 *
 * Target Traffic Pattern: Ramping to 3x nominal load (e.g. 3000 RPS) and holding at peak
 * Target Environment: Staging / Pre-prod / Load Env
 * Outputs for slo-input.yaml: peak_qps, producer_throughput_rps, aimd_*_frac, peak_qps_per_pod
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
  refreshToken,
  getActiveJwt,
} from "../lib/api.ts";
import exec from "k6/execution";

export const options: Options = {
  scenarios: {
    stress_test: CONFIG.workloads.stress as RampingArrivalRateScenario,
  },
  thresholds: CONFIG.thresholds.stress,
};

export default function (): void {
  const rand: number = Math.random();
  const vu: number = exec.vu.idInTest;
  const iter: number = exec.scenario.iterationInTest;

  if (rand < 0.7) {
    // 70% Transfers
    const { fromWallet, toWallet, jwt, apiKey, merchantIdx }: WalletPair =
      selectWalletPair(vu, iter);
    const activeJwt = getActiveJwt(merchantIdx, jwt);
    const res = submitTransfer(fromWallet, toWallet, 100, activeJwt);
    if (res.status === 401) {
      refreshToken(merchantIdx, apiKey);
    }
  } else {
    // 30% Deposits (continually replenishing wallet balances to prevent insufficient funds)
    const { walletId, jwt, apiKey, merchantIdx }: DepositContext =
      selectDepositWallet(vu, iter);
    const activeJwt = getActiveJwt(merchantIdx, jwt);
    const res = submitDeposit(walletId, 5000, activeJwt);
    if (res.status === 401) {
      refreshToken(merchantIdx, apiKey);
    }
  }
}
