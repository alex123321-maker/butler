package com.example.data.local

import android.content.Context
import androidx.room.Database
import androidx.room.Room
import androidx.room.RoomDatabase

@Database(
    entities = [
        QueuedNotificationEntity::class,
        AppFilterRuleEntity::class,
        DiagnosticLogEntity::class
    ],
    version = 1,
    exportSchema = false
)
abstract class ButlerDatabase : RoomDatabase() {

    abstract fun butlerDao(): ButlerDao

    companion object {
        private const val DATABASE_NAME = "butler_companion_queue.db"

        @Volatile
        private var INSTANCE: ButlerDatabase? = null

        fun getInstance(context: Context): ButlerDatabase {
            return INSTANCE ?: synchronized(this) {
                INSTANCE ?: Room.databaseBuilder(
                    context.applicationContext,
                    ButlerDatabase::class.java,
                    DATABASE_NAME
                )
                    .build()
                    .also { INSTANCE = it }
            }
        }

        fun createInMemory(context: Context): ButlerDatabase {
            return Room.inMemoryDatabaseBuilder(
                context.applicationContext,
                ButlerDatabase::class.java
            )
                .allowMainThreadQueries()
                .build()
        }
    }
}
