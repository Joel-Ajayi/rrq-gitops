/**
 * SCENARIO: Breakpoint Test (Over-Saturation & Circuit Breaker Fault Injection)
 * PRIMARY QUESTION ANSWERED: "Where does the cluster collapse, what component fails first, and do circuit breakers trip to isolate DBs?"
 * 
 * Target Traffic Pattern: Unlimited continuous ramping (50 -> 1500+ RPS) until error rate hits 100% or system crashes
 * Target Environment: Isolated Test / Benchmark Env
 * Outputs for slo-input.yaml: circuit_breaker.error_threshold, min_requests, max_fails, half_open_probes, system recovery time
 */

import { Options, RampingArrivalRateScenario } from 'k6/options';
import { CONFIG } from '../lib/config.ts';
import { selectWalletPair, selectDepositWallet, WalletPair, DepositContext } from '../lib/data.ts';
import { submitTransfer, submitDeposit } from '../lib/api.ts';

import exec from 'k6/execution';

export const options: Options = {
  scenarios: {
    breakpoint_capacity_test: CONFIG.workloads.breakpoint as RampingArrivalRateScenario,
  },
  thresholds: CONFIG.thresholds.breakpoint,
};

export default function (): void {
  // 80% Transfers, 20% Wallet Deposits (Matches production workload ratio)
  const isDeposit: boolean = Math.random() < 0.20;

  if (isDeposit) {
    const vu: number = exec.vu.idInTest;
    const iter: number = exec.scenario.iterationInTest;
    const { walletId, jwt }: DepositContext = selectDepositWallet(vu, iter);
    submitDeposit(walletId, 500, jwt);
  } else {
    const vu: number = exec.vu.idInTest;
    const iter: number = exec.scenario.iterationInTest;
    const { fromWallet, toWallet, jwt }: WalletPair = selectWalletPair(vu, iter);
    submitTransfer(fromWallet, toWallet, 100, jwt);
  }
}
