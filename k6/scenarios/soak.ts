import { Options, ConstantArrivalRateScenario } from 'k6/options';
import { CONFIG } from '../lib/config.ts';
import { selectWalletPair, selectDepositWallet, WalletPair, DepositContext } from '../lib/data.ts';
import { submitTransfer, submitDeposit } from '../lib/api.ts';

import exec from 'k6/execution';

export const options: Options = {
  scenarios: {
    soak_test: <ConstantArrivalRateScenario>CONFIG.workloads.soak,
  },
  thresholds: CONFIG.thresholds.soak,
};

export default function (): void {
  // 85% Transfers, 15% Wallet Deposits
  const isDeposit: boolean = Math.random() < 0.15;

  if (isDeposit) {
    const vu: number = exec.vu.idInTest;
    const iter: number = exec.scenario.iterationInTest;
    const { walletId, jwt }: DepositContext = selectDepositWallet(vu, iter);
    submitDeposit(walletId, 250, jwt);
  } else {
    const vu: number = exec.vu.idInTest;
    const iter: number = exec.scenario.iterationInTest;
    const { fromWallet, toWallet, jwt }: WalletPair = selectWalletPair(vu, iter);
    submitTransfer(fromWallet, toWallet, 100, jwt);
  }
}
