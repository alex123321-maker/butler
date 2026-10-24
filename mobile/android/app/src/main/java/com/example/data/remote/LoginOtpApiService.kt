package com.example.data.remote

import com.squareup.moshi.Json
import retrofit2.Response
import retrofit2.http.Body
import retrofit2.http.Header
import retrofit2.http.POST

class LoginOtpRequest(
    @param:Json(name = "event_id") val eventId: String,
    @param:Json(name = "package_name") val packageName: String,
    val sender: String,
    val code: String,
    @param:Json(name = "posted_at_epoch_ms") val postedAt: Long
) { override fun toString() = "LoginOtpRequest([redacted])" }
data class LoginOtpResponse(val accepted: Boolean)

interface LoginOtpApiService {
    @POST("/api/v1/mobile/otp/login")
    suspend fun submit(
        @Header("Authorization") bearer: String,
        @Header("X-Butler-Device-Id") device: String,
        @Body request: LoginOtpRequest
    ): Response<LoginOtpResponse>
}
