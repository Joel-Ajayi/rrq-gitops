/**
 * SCENARIO: Smoke Test
 * PRIMARY QUESTION ANSWERED: "Is the deployment functional, and what is the baseline DB write count per request?"
 * 
 * Target Traffic Pattern: Very low, constant rate (e.g. 5 RPS for 30s)
 * Target Environment: Dev / Staging / Production
 * Outputs for slo-input.yaml: writes_per_message
 */

import { Options, ConstantArrivalRateScenario } from 'k6/options';
import { CONFIG } from '../lib/config.ts';
import { selectWalletPair, WalletPair } from '../lib/data.ts';
import { submitTransfer } from '../lib/api.ts';
import exec from 'k6/execution';

export const options: Options = {
  scenarios: {
    smoke_test: CONFIG.workloads.smoke as ConstantArrivalRateScenario,
  },
  thresholds: CONFIG.thresholds.smoke,
};

export default function (): void {
  const vu: number = exec.vu.idInTest;
  const iter: number = exec.scenario.iterationInTest;
  const { fromWallet, toWallet, jwt }: WalletPair = selectWalletPair(vu, iter);
  submitTransfer(fromWallet, toWallet, 100, jwt);
}
