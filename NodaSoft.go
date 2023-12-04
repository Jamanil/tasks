package main

import (
	"fmt"
	"math/rand"
	"time"
)

const (
	taskCompleted = "task has been finished successful"
	taskFailed    = "task has been failed"
)

// ЗАДАНИЕ:
// * сделать из плохого кода хороший;
// * важно сохранить логику появления ошибочных тасков;
// * сделать правильную мультипоточность обработки заданий.
// Обновленный код отправить через merge-request.

// приложение эмулирует получение и обработку тасков, пытается и получать и обрабатывать в многопоточном режиме
// В конце должно выводить успешные таски и ошибки выполнены остальных тасков

// A Task represents a meaninglessness of our life
type Task struct {
	id         int
	createdAt  string // время создания
	finishedAt string // время выполнения
	result     []byte
}

func main() {
	newTasks := make(chan Task, 10)
	processedTasks := make(chan Task)
	completedTasks := make(chan Task)
	failedTasks := make(chan error)

	go createTasks(newTasks)
	go processTasks(newTasks, processedTasks)
	go sortTask(processedTasks, completedTasks, failedTasks)

	completedTasksIDs := make([]int, 0)
	failedTasksErrors := make([]error, 0)

	go getCompletedTasksIds(completedTasks, &completedTasksIDs)
	go getTasksErrors(failedTasks, &failedTasksErrors)

	time.Sleep(time.Second * 3)

	printSlice(failedTasksErrors, "Errors:")
	printSlice(completedTasksIDs, "Done tasks:")
}

func createTasks(tasks chan<- Task) {
	for {
		timeFormat := time.Now().Format(time.RFC3339)
		go func() {
			if time.Now().Nanosecond()%2 > 0 { // вот такое условие появления ошибочных тасков
				timeFormat = "Some error occupied"
			}
			tasks <- Task{createdAt: timeFormat, id: int(time.Now().Unix())} // передаем таск на выполнение
		}()
	}
}

func processTasks(tasks <-chan Task, processedTasks chan<- Task) {
	for range tasks {
		go func() {
			task := <-tasks
			tt, _ := time.Parse(time.RFC3339, task.createdAt)
			if tt.After(time.Now().Add(-20 * time.Second)) {
				task.result = []byte(taskCompleted)
			} else {
				// Следует заменить сообщение об ошибке "something went wrong" чем-то более информативным
				task.result = []byte(taskFailed)
			}
			task.finishedAt = time.Now().Format(time.RFC3339Nano)
			processedTasks <- task
			time.Sleep(time.Millisecond * time.Duration(rand.Intn(300)))
		}()
	}
	close(processedTasks)
}

func sortTask(tasks <-chan Task, completedTasks chan<- Task, failedTasks chan<- error) {
	for range tasks {
		task := <-tasks
		// Плохой вариант проверки. Возможно, следует добавить поле status в тип Task или изменить алгоритм иным образом
		if string(task.result) == taskCompleted {
			completedTasks <- task
		} else {
			failedTasks <- fmt.Errorf("task id %d time %s, error %s", task.id, task.createdAt, task.result)
		}
	}
	close(completedTasks)
	close(failedTasks)
}

func getCompletedTasksIds(completedTasks <-chan Task, ids *[]int) {
	for range completedTasks {
		task := <-completedTasks
		*ids = append(*ids, task.id)
	}
}

func getTasksErrors(failedTasks <-chan error, errors *[]error) {
	for range failedTasks {
		*errors = append(*errors, <-failedTasks)
	}
}

func printSlice[A any](slice []A, header string) {
	if len(slice) > 0 {
		fmt.Println(header)
		for _, a := range slice {
			fmt.Println(a)
		}
	}
}
