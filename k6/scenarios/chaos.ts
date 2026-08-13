import { Options, ConstantArrivalRateScenario, PerVUIterationsScenario } from 'k6/options';
import { CONFIG } from '../lib/config.ts';
import { selectWalletPair, selectDepositWallet, WalletPair, DepositContext } from '../lib/data.ts';
import { submitTransfer, submitDeposit, getWalletBalance, getActiveJwt, refreshToken } from '../lib/api.ts';
import { PodDisruptor } from 'k6/x/disruptor';

import exec from 'k6/execution';

export const options: Options = {
  scenarios: {
    chaos_test: <ConstantArrivalRateScenario>CONFIG.workloads.chaos,
    inject_faults: <PerVUIterationsScenario>{
      executor: 'per-vu-iterations',
      exec: 'faultInjection',
      vus: 1,
      iterations: 1,
      startTime: '10s', // Inject faults 10s into the test
    }
  },
  thresholds: CONFIG.thresholds.chaos,
};

export function faultInjection() {
  const disruptor = new PodDisruptor({
    namespace: 'rrq',
    select: {
      labels: {
        'app.kubernetes.io/name': 'core-api',
      },
    },
  });
  // Inject 500ms delay and 10% 500 errors for 60 seconds
  disruptor.injectHTTPFaults({ averageDelay: '500ms', errorRate: 0.1, errorCode: 500 }, '60s');
}

export default function (): void {
  const rand: number = Math.random();
  const vu: number = exec.vu.idInTest;
  const iter: number = exec.scenario.iterationInTest;

  if (rand < 0.30) {
    // 30% Wallet Deposits
    const { walletId, jwt, apiKey, merchantIdx }: DepositContext = selectDepositWallet(vu, iter);
    const activeJwt = getActiveJwt(merchantIdx, jwt);
    const res = submitDeposit(walletId, 500, activeJwt);
    if (res.status === 401) {
      refreshToken(merchantIdx, apiKey);
    }
  } else if (rand < 0.40) {
    // 10% Wallet Balances (Reads)
    const { walletId, jwt, apiKey, merchantIdx }: DepositContext = selectDepositWallet(vu, iter);
    const activeJwt = getActiveJwt(merchantIdx, jwt);
    const res = getWalletBalance(walletId, activeJwt);
    if (res.status === 401) {
      refreshToken(merchantIdx, apiKey);
    }
  } else {
    // 60% Transfers
    const { fromWallet, toWallet, jwt, apiKey, merchantIdx }: WalletPair = selectWalletPair(vu, iter);
    const activeJwt = getActiveJwt(merchantIdx, jwt);
    const res = submitTransfer(fromWallet, toWallet, 100, activeJwt);
    if (res.status === 401) {
      refreshToken(merchantIdx, apiKey);
    }
  }
}
