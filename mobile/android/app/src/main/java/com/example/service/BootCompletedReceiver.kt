package com.example.service

import android.content.BroadcastReceiver
import android.content.Context
import android.content.Intent
import com.example.ButlerApplication
import kotlinx.coroutines.launch

/**
 * Recovers interrupted queue items and initiates pending batch synchronization
 * after device reboot (`BOOT_COMPLETED`) or app update (`MY_PACKAGE_REPLACED`).
 */
class BootCompletedReceiver : BroadcastReceiver() {

    override fun onReceive(context: Context, intent: Intent?) {
        val action = intent?.action ?: return
        if (action != Intent.ACTION_BOOT_COMPLETED && action != Intent.ACTION_MY_PACKAGE_REPLACED) {
            return
        }

        val app = (context.applicationContext as? ButlerApplication) ?: ButlerApplication.instance ?: return
        val pendingResult = goAsync()
        app.appScope.launch {
            try {
                app.syncRepository.ensureDefaultAppRulesSeeded()
                app.syncRepository.logDiagnostic(
                    level = "INFO",
                    stage = "QUEUE",
                    code = "BOOT_RECOVERY_SYNC",
                    summaryRu = "Проверка локальной очереди и возобновление доставки после перезагрузки/обновления ($action)"
                )
                app.syncRepository.syncPendingQueueBatch()
            } finally {
                pendingResult.finish()
            }
        }
    }
}
