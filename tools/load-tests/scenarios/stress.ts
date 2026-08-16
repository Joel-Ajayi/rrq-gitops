/**
 * SCENARIO: Stress Test (Peak Capacity & Backpressure)
 * PRIMARY QUESTION ANSWERED: "Can horizontal autoscaling (KEDA/HPA) and backpressure mechanisms handle projected peak traffic?"
 * 
 * Target Traffic Pattern: Ramping to 3x nominal load (e.g. 3000 RPS) and holding at peak
 * Target Environment: Staging / Pre-prod / Load Env
 * Outputs for slo-input.yaml: peak_qps, producer_throughput_rps, aimd_*_frac, peak_qps_per_pod
 */

import { Options, RampingArrivalRateScenario } from 'k6/options';
import { CONFIG } from '../lib/config.ts';
import { selectWalletPair, WalletPair } from '../lib/data.ts';
import { submitTransfer } from '../lib/api.ts';
import exec from 'k6/execution';

export const options: Options = {
  scenarios: {
    stress_test: CONFIG.workloads.stress as RampingArrivalRateScenario,
  },
  thresholds: CONFIG.thresholds.stress,
};

export default function (): void {
  const vu: number = exec.vu.idInTest;
  const iter: number = exec.scenario.iterationInTest;
  const { fromWallet, toWallet, jwt }: WalletPair = selectWalletPair(vu, iter);
  submitTransfer(fromWallet, toWallet, 100, jwt);
}
