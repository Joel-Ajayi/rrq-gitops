import { Options, ConstantArrivalRateScenario } from 'k6/options';
import { CONFIG } from '../lib/config.ts';
import { selectWalletPair, WalletPair } from '../lib/data.ts';
import { submitTransfer } from '../lib/api.ts';
import exec from 'k6/execution';

export const options: Options = {
  scenarios: {
    chaos_test: <ConstantArrivalRateScenario>CONFIG.workloads.chaos,
  },
  thresholds: CONFIG.thresholds.chaos,
};

export default function (): void {
  const vu: number = exec.vu.idInTest;
  const iter: number = exec.scenario.iterationInTest;
  const { fromWallet, toWallet, jwt }: WalletPair = selectWalletPair(vu, iter);
  submitTransfer(fromWallet, toWallet, 100, jwt);
}
