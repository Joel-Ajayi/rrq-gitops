import { Options, RampingArrivalRateScenario } from 'k6/options';
import { CONFIG } from '../lib/config.ts';
import { selectWalletPair, WalletPair } from '../lib/data.ts';
import { submitTransfer } from '../lib/api.ts';
import exec from 'k6/execution';

export const options: Options = {
  scenarios: {
    spike_test: <RampingArrivalRateScenario>CONFIG.workloads.spike,
  },
  thresholds: CONFIG.thresholds.spike,
};

export default function (): void {
  const vu: number = exec.vu.idInTest;
  const iter: number = exec.scenario.iterationInTest;
  const { fromWallet, toWallet, jwt }: WalletPair = selectWalletPair(vu, iter);
  submitTransfer(fromWallet, toWallet, 100, jwt);
}
