package com.example.data.remote

import com.example.model.DeviceStatusResponseDto
import com.example.model.DisconnectRequestDto
import com.example.model.NotificationBatchRequestDto
import com.example.model.NotificationBatchResponseDto
import com.example.model.PairExchangeRequestDto
import com.example.model.PairExchangeResponseDto
import com.example.model.PairPollRequestDto
import com.example.model.PairPollResponseDto
import com.example.model.PairRequestInitDto
import com.example.model.PairRequestInitResponseDto
import retrofit2.Response
import retrofit2.http.Body
import retrofit2.http.GET
import retrofit2.http.Header
import retrofit2.http.POST

/**
 * Retrofit HTTP interface for the Butler Go Backend Mobile Companion API v1.
 */
interface ButlerMobileApiService {

    @POST("/api/v1/mobile/pair/exchange")
    suspend fun exchangePairingCode(
        @Body request: PairExchangeRequestDto
    ): Response<PairExchangeResponseDto>

    @POST("/api/v1/mobile/pair/request")
    suspend fun initiatePhonePairing(
        @Body request: PairRequestInitDto
    ): Response<PairRequestInitResponseDto>

    @POST("/api/v1/mobile/pair/poll")
    suspend fun pollPhonePairingApproval(
        @Body request: PairPollRequestDto
    ): Response<PairPollResponseDto>

    @GET("/api/v1/mobile/status")
    suspend fun getDeviceStatus(
        @Header("Authorization") bearerToken: String,
        @Header("X-Butler-Device-Id") deviceId: String,
        @Header("X-Butler-Schema-Version") schemaVersion: Int = 1
    ): Response<DeviceStatusResponseDto>

    @POST("/api/v1/mobile/notifications/batch")
    suspend fun postNotificationBatch(
        @Header("Authorization") bearerToken: String,
        @Header("X-Butler-Device-Id") deviceId: String,
        @Header("X-Butler-Schema-Version") schemaVersion: Int = 1,
        @Body batch: NotificationBatchRequestDto
    ): Response<NotificationBatchResponseDto>

    @POST("/api/v1/mobile/disconnect")
    suspend fun disconnectDevice(
        @Header("Authorization") bearerToken: String,
        @Header("X-Butler-Device-Id") deviceId: String,
        @Body request: DisconnectRequestDto
    ): Response<Unit>
}
