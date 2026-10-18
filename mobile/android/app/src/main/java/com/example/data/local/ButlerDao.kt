package com.example.data.local

import androidx.room.Dao
import androidx.room.Insert
import androidx.room.OnConflictStrategy
import androidx.room.Query
import kotlinx.coroutines.flow.Flow

@Dao
interface ButlerDao {

    // --- Queue Queries ---

    @Query("SELECT * FROM queued_notification_events ORDER BY observedAtEpochMs DESC LIMIT 250")
    fun observeRecentQueueEvents(): Flow<List<QueuedNotificationEntity>>

    @Query("SELECT COUNT(*) FROM queued_notification_events WHERE deliveryState IN ('PENDING', 'IN_FLIGHT')")
    fun observePendingCount(): Flow<Int>

    @Query("SELECT COUNT(*) FROM queued_notification_events WHERE deliveryState = 'DELIVERED'")
    fun observeDeliveredCount(): Flow<Int>

    @Query("SELECT COUNT(*) FROM queued_notification_events WHERE deliveryState IN ('CONFLICT_ERROR', 'PERMANENT_ERROR')")
    fun observeErrorCount(): Flow<Int>

    @Query("SELECT COALESCE(SUM(payloadSizeBytes), 0) FROM queued_notification_events")
    fun observeTotalQueueBytes(): Flow<Long>

    @Query("SELECT COUNT(*) FROM queued_notification_events")
    suspend fun getTotalEventCount(): Int

    @Query("SELECT COALESCE(SUM(payloadSizeBytes), 0) FROM queued_notification_events")
    suspend fun getTotalQueueBytes(): Long

    @Query(
        """
        SELECT * FROM queued_notification_events
        WHERE notificationKey = :notificationKey
        ORDER BY observedAtEpochMs DESC, rowid DESC
        LIMIT 1
        """
    )
    suspend fun getLatestActiveEventForKey(notificationKey: String): QueuedNotificationEntity?

    @Query("SELECT COUNT(*) FROM queued_notification_events WHERE observedAtEpochMs < :cutoff AND deliveryState != 'DELIVERED'")
    suspend fun countUndeliveredBefore(cutoff: Long): Int

    @Insert(onConflict = OnConflictStrategy.ABORT)
    suspend fun insertQueuedEvent(entity: QueuedNotificationEntity)

    @Query("SELECT * FROM queued_notification_events WHERE deviceEventId = :deviceEventId LIMIT 1")
    suspend fun getEventById(deviceEventId: String): QueuedNotificationEntity?

    @Query(
        """
        SELECT * FROM queued_notification_events
        WHERE deliveryState = 'PENDING' AND nextAttemptAtEpochMs <= :nowEpochMs
        ORDER BY observedAtEpochMs ASC
        LIMIT :limit
        """
    )
    suspend fun getReadyBatchForSync(nowEpochMs: Long, limit: Int): List<QueuedNotificationEntity>

    @Query(
        """
        UPDATE queued_notification_events
        SET deliveryState = 'IN_FLIGHT'
        WHERE deviceEventId IN (:ids)
        """
    )
    suspend fun markBatchInFlight(ids: List<String>)

    @Query(
        """
        UPDATE queued_notification_events
        SET deliveryState = 'PENDING'
        WHERE deliveryState = 'IN_FLIGHT'
        """
    )
    suspend fun recoverStaleInFlightEvents(): Int

    @Query(
        """
        UPDATE queued_notification_events
        SET deliveryState = 'DELIVERED',
            deliveredAtEpochMs = :deliveredAtMs,
            lastErrorCode = NULL,
            lastErrorReason = NULL
        WHERE deviceEventId IN (:ids)
        """
    )
    suspend fun markEventsDelivered(ids: List<String>, deliveredAtMs: Long)

    @Query(
        """
        UPDATE queued_notification_events
        SET deliveryState = 'CONFLICT_ERROR',
            lastErrorCode = :errorCode,
            lastErrorReason = :reason
        WHERE deviceEventId = :deviceEventId
        """
    )
    suspend fun markEventConflictError(deviceEventId: String, errorCode: String, reason: String)

    @Query(
        """
        UPDATE queued_notification_events
        SET deliveryState = 'PENDING',
            attemptCount = attemptCount + 1,
            nextAttemptAtEpochMs = :nextAttemptAtMs,
            lastErrorCode = :errorCode,
            lastErrorReason = :reason
        WHERE deviceEventId IN (:ids)
        """
    )
    suspend fun scheduleRetryForBatch(
        ids: List<String>,
        nextAttemptAtMs: Long,
        errorCode: String,
        reason: String
    )

    @Query("DELETE FROM queued_notification_events WHERE observedAtEpochMs < :cutoffEpochMs")
    suspend fun deleteEventsOlderThan(cutoffEpochMs: Long): Int

    @Query(
        """
        DELETE FROM queued_notification_events
        WHERE deviceEventId IN (
            SELECT deviceEventId FROM queued_notification_events
            WHERE deliveryState = 'DELIVERED'
            ORDER BY observedAtEpochMs ASC
            LIMIT :limit
        )
        """
    )
    suspend fun purgeOldestDeliveredEvents(limit: Int): Int

    @Query(
        """
        DELETE FROM queued_notification_events
        WHERE deviceEventId IN (
            SELECT deviceEventId FROM queued_notification_events
            ORDER BY observedAtEpochMs ASC
            LIMIT :limit
        )
        """
    )
    suspend fun evictOldestEventsOnOverflow(limit: Int): Int

    @Query("DELETE FROM queued_notification_events WHERE deliveryState = 'DELIVERED'")
    suspend fun clearAllDeliveredEvents(): Int

    @Query("DELETE FROM queued_notification_events")
    suspend fun clearEntireQueue()

    // --- App Filter Rules ---

    @Query("SELECT * FROM app_filter_rules ORDER BY isSensitiveSource DESC, appLabel ASC")
    fun observeAppFilterRules(): Flow<List<AppFilterRuleEntity>>

    @Query("SELECT * FROM app_filter_rules")
    suspend fun getAllAppFilterRules(): List<AppFilterRuleEntity>

    @Query("SELECT * FROM app_filter_rules WHERE packageName = :packageName LIMIT 1")
    suspend fun getAppFilterRule(packageName: String): AppFilterRuleEntity?

    @Insert(onConflict = OnConflictStrategy.REPLACE)
    suspend fun upsertAppFilterRule(rule: AppFilterRuleEntity)

    @Insert(onConflict = OnConflictStrategy.IGNORE)
    suspend fun insertDefaultAppFilterRules(rules: List<AppFilterRuleEntity>)

    // --- Zero-PII Diagnostic Logs ---

    @Query("SELECT * FROM diagnostic_logs ORDER BY timestampEpochMs DESC LIMIT 150")
    fun observeDiagnosticLogs(): Flow<List<DiagnosticLogEntity>>

    @Insert(onConflict = OnConflictStrategy.REPLACE)
    suspend fun insertDiagnosticLog(log: DiagnosticLogEntity)

    @Query(
        """
        DELETE FROM diagnostic_logs
        WHERE id NOT IN (
            SELECT id FROM diagnostic_logs
            ORDER BY timestampEpochMs DESC
            LIMIT 300
        )
        """
    )
    suspend fun trimDiagnosticLogs()

    @Query("DELETE FROM diagnostic_logs")
    suspend fun clearDiagnosticLogs()
}
