/**
 * SCENARIO: Spike Test (Sudden Surge & Buffer Sizing)
 * PRIMARY QUESTION ANSWERED: "Do in-memory channel buffers and thread headroom absorb sudden 10x traffic surges while pods scale?"
 * 
 * Target Traffic Pattern: Sudden violent jump (e.g. 100 RPS -> 2500 RPS in 30s, then dropping back)
 * Target Environment: Staging / Load Env
 * Outputs: Validates autoscale lag, http_headroom, and consumer_partition_size buffer depth.
 */

import { Options, RampingArrivalRateScenario } from 'k6/options';
import { CONFIG } from '../lib/config.ts';
import { selectWalletPair, WalletPair } from '../lib/data.ts';
import { submitTransfer } from '../lib/api.ts';
import exec from 'k6/execution';

export const options: Options = {
  scenarios: {
    spike_test: CONFIG.workloads.spike as RampingArrivalRateScenario,
  },
  thresholds: CONFIG.thresholds.spike,
};

export default function (): void {
  const vu: number = exec.vu.idInTest;
  const iter: number = exec.scenario.iterationInTest;
  const { fromWallet, toWallet, jwt }: WalletPair = selectWalletPair(vu, iter);
  submitTransfer(fromWallet, toWallet, 100, jwt);
}
